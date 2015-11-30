package wemdigo

import (
	"time"

	"github.com/gorilla/websocket"
)

type Link struct {
	ws    *websocket.Conn // Underlying Gorilla websocket connection.
	id    string          // Peer identifier for the websocket.
	peers cmap            // List of other connections the socket can talk to.
	send  chan *Message   // Messages to write to the websocket.
	mid   *Middle         // Middle instance to which the Link belongs.
}

func (l *Link) setReadDeadline() {
	var t time.Time
	if l.mid.conf.PongWait != 0 {
		t = time.Now().Add(l.mid.conf.PongWait)
	}
	l.ws.SetReadDeadline(t)
}

func (l *Link) updatePeers() {
	l.peers.Lock()
	for id := range l.peers.m {
		// Check to see that each peer is registered with the Middle.
		if _, ok := l.mid.Links.get(id); !ok {
			delete(l.peers.m, id)
		}
	}
	l.peers.Unlock()
}

// FIXME: add this func
// Conn returns the underlying Gorilla websocket connection.
// func (l *Link) Conn() *websocket.Conn {
// 	return ws.Conn
// }

// isAlone checks which of the Link's peers are still registred with
// the middle instance, removes any unregistered Links, and reports
// whether any peers still exist.
func (l *Link) isAlone() bool {
	l.updatePeers()
	return l.peers.isEmpty()
}

// readLoop pumps messages from the websocket Link to the Middle.
func (l *Link) readLoop() {
	defer func() {
		// Check first whether the Link is still associated to the middle.

		// FIXME we might not need to check the map first if we only ever
		// delete a Link when the Middle sees an unregistration.
		if _, ok := l.mid.Links.get(l.id); ok {
			dlog("Link with id = %s sending to unregister chan.", l.id)
			l.mid.unregister <- l
		}
		l.ws.Close()
	}()

	ponghandler := func(appData string) error {
		l.setReadDeadline()
		dlog("Link with id = %s saw a pong.", l.id)
		return nil
	}

	l.ws.SetPongHandler(ponghandler)

	for {
		if l.isAlone() {
			return
		}

		mt, data, err := l.ws.ReadMessage()
		if err != nil {
			return
		}
		dlog("Link with id = %s read a message", l.id)

		msg := &Message{Type: mt, Data: data, origin: l}
		l.mid.raw <- msg
	}
}

func (l *Link) write(messageType int, data []byte) error {
	l.ws.SetWriteDeadline(time.Now().Add(l.mid.conf.WriteWait))
	return l.ws.WriteMessage(messageType, data)
}

// writeLoop pumps messages from the Middle to the websocket Link.
func (l *Link) writeLoop() {
	ticker := time.NewTicker(l.mid.conf.PingPeriod)
	defer func() {
		ticker.Stop()
		l.ws.Close()
		dlog("Link with id = %s no longer writing messages.", l.id)
		// Drain any remaining messages that the Middle might send.
		// When the Link is unregistered, the Middle will close the send chan.
		for range l.send {
			continue
		}
	}()
	for {
		select {
		case msg, ok := <-l.send:
			if !ok {
				l.write(websocket.CloseMessage, nil)
				return
			}

			if msg.close {
				return
			}

			if err := l.write(msg.Type, msg.Data); err != nil {
				return
			}

		case <-ticker.C:
			if err := l.write(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (l *Link) run() {
	go l.writeLoop()
	go l.readLoop()
}

// ID reports the Link's user specified ID.  This ID corresponds to
// it's ID in a Middle instance's map of Links.
func (l *Link) ID() string {
	return l.id
}

// Peers reports the ids of the currently active peers for this Link.
func (l *Link) Peers() []string {
	l.updatePeers()
	return l.peers.linkIds()
}
