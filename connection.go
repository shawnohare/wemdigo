package wemdigo

import (
	"log"
	"time"

	"github.com/gorilla/websocket"
)

type connection struct {
	ws   *websocket.Conn
	h    MessageHandler
	name string
	send chan *Message // Channel to receive messages from Middle.
	conf *Config       // Middleware conf so we may access keep alive times.
}

func (c *connection) write(msg *Message) error {
	c.ws.SetWriteDeadline(time.Now().Add(c.conf.WriteWait))
	return c.ws.WriteMessage(msg.Type, msg.Data)
}

// Process incoming messages from the websocket connection and
// send the result to a Middle hub for redirection.
func (c *connection) readMessages(middlewareChan chan<- *Message) {
	ws := c.ws
	pongHandler := func(string) error {
		// FIXME: remove logging
		log.Println("Connection", c.name, "received a pong message")
		// Reset deadline.
		ws.SetReadDeadline(time.Now().Add(c.conf.PongWait))
		return nil
	}

	defer func() {
		ws.WriteMessage(websocket.CloseMessage, []byte{})
		ws.Close()
	}()

	ws.SetReadDeadline(time.Now().Add(c.conf.PongWait))
	ws.SetPongHandler(pongHandler)
	for {
		mt, raw, err := ws.ReadMessage()
		if err != nil {
			break
		}

		msg, pass, err := c.h(&Message{mt, raw, c.name})

		// Close the socket in case of an error.
		if err != nil {
			// FIXME: do what in case the handler errors?
			log.Println("Handler error:", err)
			break
		}

		// Only pass messages the handler deems worthy.
		if !pass {
			continue
		}

		// Ensure the message origin is preserved so that writers of
		// message handlers can ignore this parameter if they choose.
		msg.Origin = c.name
		middlewareChan <- msg
	}
}

// writeMessages pumps messages from the Middle hub to the  websocket.
// It also keeps the underlying websocket connection alive by sending pings.
func (c *connection) writeMessages() {
	ws := c.ws
	ticker := time.NewTicker(c.conf.PingPeriod)

	defer func() {
		ticker.Stop()
		ws.Close()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			if !ok {
				ws.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.write(msg); err != nil {
				return
			}

		case <-ticker.C:
			control := &Message{websocket.PingMessage, nil, ""}
			if err := c.write(control); err != nil {
				return
			}
		}
	}
}

func newConnection(ws *websocket.Conn, h MessageHandler, name string, conf *Config) *connection {
	return &connection{
		ws:   ws,
		h:    h,
		name: name,
		send: make(chan *Message),
		conf: conf,
	}
}
