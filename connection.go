package wemdigo

import (
	"log"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/gorilla/websocket"
)

type connection struct {
	ws   *websocket.Conn
	h    MessageHandler
	name string
	send chan *Message // Channel to of messages to write.
	proc chan *Message // Channel to process messages with the handler.
	conf *Config       // Middleware conf so we may access keep alive times.
}

func (c *connection) write(msg *Message) error {
	c.ws.SetWriteDeadline(time.Now().Add(c.conf.WriteWait))
	return c.ws.WriteMessage(msg.Type, msg.Data)
}

// processMessages ingests messages in the proc channel, processes them,
// and sends good messages to the Middle (via the out channel) for
// re-direction.
func (c *connection) processMessages(out chan<- *Message) {
	defer c.ws.Close()
	for msg := range c.proc {
		log.Println("received message on proc chan")
		spew.Dump(msg)
		pmsg, pass, err := c.h(msg)

		// Close the socket in case of an error.
		if err != nil {
			control := &Message{websocket.CloseMessage, nil, c.name}
			c.send <- control
			log.Println("Handler error:", err)
			return
		}

		// Only pass messages the handler deems worthy.
		if !pass {
			continue
		}

		// Ensure the message origin is preserved so that writers of
		// message handlers can ignore this parameter if they choose.
		pmsg.Origin = c.name
		out <- pmsg
	}
}

// Read incoming messages from the websocket connection and
// push them onto an internal channel for further processing.
// This ensures that processing messages does not block reading.
func (c *connection) readMessages() {
	ws := c.ws
	pongHandler := func(string) error {
		// log.Println("Connection", c.name, "received a pong message")
		// Reset deadline.
		ws.SetReadDeadline(time.Now().Add(c.conf.PongWait))
		return nil
	}

	defer func() {
		ws.Close()
		close(c.proc)
	}()

	ws.SetReadDeadline(time.Now().Add(c.conf.PongWait))
	ws.SetPongHandler(pongHandler)
	for {
		mt, raw, err := ws.ReadMessage()
		if err != nil {
			control := &Message{websocket.CloseMessage, nil, c.name}
			c.send <- control
			break
		}

		log.Println("sending message to proc channel")
		c.proc <- &Message{mt, raw, c.name}
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
			// FIXME
			log.Println(c.name, "sending a ping.")
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
		proc: make(chan *Message),
		conf: conf,
	}
}
