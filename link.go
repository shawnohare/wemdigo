package wemdigo

import (
	"log"
	"time"

	"github.com/gorilla/websocket"
)

type link struct {
	ws   *Conn         // Underlying Gorilla websocket connection.
	id   string        // Peer identifier for the websocket.
	send chan *Message // Messages to write to the websocket.
	mid  *Middle       // Middle instance to which the link belongs.
}

func (l *link) setReadDeadline() {
	var t time.Time
	if l.mid.conf.PongWait != 0 {
		t = time.Now().Add(l.mid.conf.PongWait)
	}
	l.ws.SetReadDeadline(t)
}

// readLoop pumps messages from the websocket link to the Middle.
func (l *link) readLoop() {
	defer func() {
		// Check first whether the link is still associated to the middle.

		// FIXME we might not need to check the map first if we only ever
		// delete a link when the Middle sees an unregistration.
		if _, ok := l.mid.links[l.id]; ok {
			log.Println("[wemdigo] Link", l.id, "sending to unregister chan.")
			l.mid.unregister <- l
		}
		l.ws.Close()
	}()

	ponghandler := func(appData string) error {
		l.setReadDeadline()
		log.Println("[wemdigo] conn", l.id, "saw a pong.")
		return nil
	}

	l.ws.SetPongHandler(ponghandler)

	for {
		msg, err := l.ws.Read()
		if err != nil {
			return
		}
		log.Println("[wemdigo] link", l.id, "saw message", string(msg.Data))
		if msg.Origin == "" {
			msg.Origin = l.id
		}
		l.mid.raw <- msg
	}
}

func (l *link) write(msg *Message) error {
	l.ws.SetWriteDeadline(time.Now().Add(l.mid.conf.WriteWait))
	log.Printf("[wemdigo] link %s writing message %#v", l.id, msg)
	return l.ws.Write(msg)
}

func (l *link) writeRaw(messageType int, data []byte) error {
	l.ws.SetWriteDeadline(time.Now().Add(l.mid.conf.WriteWait))
	return l.ws.WriteMessage(messageType, data)
}

// writeLoop pumps messages from the Middle to the websocket link.
func (l *link) writeLoop() {
	ticker := time.NewTicker(l.mid.conf.PingPeriod)
	defer func() {
		ticker.Stop()
		l.ws.Close()
		log.Println("[wemdigo] link", l.id, "no longer writing messages.")
		// Drain any remaining messages that the Middle might send.
		// When the link is unregistered, the Middle will close the send chan.
		for range l.send {
			continue
		}
	}()
	for {
		select {
		case message, ok := <-l.send:
			if !ok {
				l.writeRaw(websocket.CloseMessage, nil)
				return
			}

			// Deal with possible control messages.
			switch message.Command() {
			case Kill:
				l.writeRaw(websocket.CloseMessage, nil)
				return
			default:
				// No command value set, so no special processing required.
				if err := l.write(message); err != nil {
					return
				}
			}

		case <-ticker.C:
			if err := l.writeRaw(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (l *link) run() {
	go l.writeLoop()
	go l.readLoop()
}
