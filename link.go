package wemdigo

import (
	"time"

	"github.com/gorilla/websocket"
)

type link struct {
	ws    *Conn               // Underlying Gorilla websocket connection.
	id    string              // Peer identifier for the websocket.
	peers map[string]struct{} // List of other connections the socket can talk to.
	send  chan *Message       // Messages to write to the websocket.
	mid   *Middle             // Middle instance to which the link belongs.
}

func (l *link) setReadDeadline() {
	var t time.Time
	if l.mid.conf.PongWait != 0 {
		t = time.Now().Add(l.mid.conf.PongWait)
	}
	l.ws.SetReadDeadline(t)
}

// isAlone checks which of the link's peers are still registred with
// the middle instance, removes any unregistered links, and reports
// whether any peers still exist.
func (l *link) isAlone() bool {
	for id := range l.peers {
		// Check to see that each peer is registered with the Middle.
		if _, ok := l.mid.links.get(id); !ok {
			delete(l.peers, id)
		}
	}
	return len(l.peers) == 0
}

// readLoop pumps messages from the websocket link to the Middle.
func (l *link) readLoop() {
	defer func() {
		// Check first whether the link is still associated to the middle.

		// FIXME we might not need to check the map first if we only ever
		// delete a link when the Middle sees an unregistration.
		if _, ok := l.mid.links.get(l.id); ok {
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

		msg, err := l.ws.Read()
		if err != nil {
			return
		}
		dlog("Link with id = %s read a message", l.id)
		if msg.Origin == "" {
			msg.Origin = l.id
		}
		l.mid.raw <- msg
	}
}

func (l *link) writeMessage(msg *Message) error {
	l.ws.SetWriteDeadline(time.Now().Add(l.mid.conf.WriteWait))
	dlog("Link with id = %s writing.", l.id)
	return l.ws.Write(msg)
}

func (l *link) write(messageType int, data []byte) error {
	l.ws.SetWriteDeadline(time.Now().Add(l.mid.conf.WriteWait))
	return l.ws.WriteMessage(messageType, data)
}

// writeLoop pumps messages from the Middle to the websocket link.
func (l *link) writeLoop() {
	ticker := time.NewTicker(l.mid.conf.PingPeriod)
	defer func() {
		ticker.Stop()
		l.ws.Close()
		dlog("Link with id = %s no longer writing messages.", l.id)
		// Drain any remaining messages that the Middle might send.
		// When the link is unregistered, the Middle will close the send chan.
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

			// Deal with possible Command messages.
			switch msg.Command() {
			case Kill:
				l.write(websocket.CloseMessage, nil)
				return
			case EncodeJSON:
				if err := l.writeMessage(msg); err != nil {
					return
				}

			default:
				// No command value set, so no special processing required.
				if err := l.write(msg.Type, msg.Data); err != nil {
					return
				}
			}

		case <-ticker.C:
			if err := l.write(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (l *link) run() {
	go l.writeLoop()
	go l.readLoop()
}
