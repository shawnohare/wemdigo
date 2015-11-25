package wemdigo

import (
	"log"
	"time"

	"github.com/gorilla/websocket"
)

// connectConfig are params that is typically shared amongst
// multiple connection instances.  It ties the connection to a particular
// Middle layer that creates it.
type connectionConfig struct {
	comm       chan *Message // Channel to communicate with the Middle hub.
	done       chan bool     // Channel to tell Middle hub to commence shutdown.
	pingPeriod time.Duration
	pongWait   time.Duration
	writeWait  time.Duration
}

type connection struct {
	ws   *websocket.Conn
	h    MessageHandler
	peer uint8
	read chan *Message // Channel to read communications from
	send chan *Message // Channel to of messages to write to peer socket.
	proc chan *Message // Channel to process messages with the handler.
	*connectionConfig
}

// close deals with all cleanup logic.
func (c *connection) close() {
	// Write a close message to the peer.
	deadline := time.Now().Add(c.writeWait)
	log.Println("Telling peer to close.")
	c.ws.WriteControl(websocket.CloseMessage, nil, deadline)
	log.Println("Closing our end.")
	c.ws.Close()
	// Close communication channels.
	close(c.proc)
	close(c.send)
}

func (c *connection) signalClose() {
	c.done <- true
}

func (c *connection) write(msg *Message) error {
	c.ws.SetWriteDeadline(time.Now().Add(c.writeWait))
	return c.ws.WriteMessage(msg.Type, msg.Data)
}

// Read incoming messages from the websocket connection and
// push them onto an internal channel for further processing.
// This ensures that processing messages does not block reading.
func (c *connection) readMessages() {
	ws := c.ws
	pongHandler := func(string) error {
		// log.Println("Connection", c.name, "received a pong message")
		// Reset deadline.
		ws.SetReadDeadline(time.Now().Add(c.pongWait))
		return nil
	}

	ws.SetReadDeadline(time.Now().Add(c.pongWait))
	ws.SetPongHandler(pongHandler)

	// Read from the peer websocket and push them to the internal read chan.
	// If a read error is encountered, send a signal to begin shutting down.
	go func() {
		for {
			mt, raw, err := ws.ReadMessage()
			if err != nil {
				break
			}

			c.read <- &Message{mt, raw, c.peer}
		}
		c.signalClose()
	}()

	// Main read loop. Handles general messages
	// Pass all messages to the processing loop until we receive a
	// control message from the Middle to stop. Note that a message
	// passed in should never be nil, but we check just in case.
	for msg := range c.read {
		c.proc <- msg
		if msg == nil || msg.Type == controlClose {
			return
		}
	}
}

// processMessages ingests messages in the proc channel, processes them,
// and sends good messages to the Middle (via the out channel) for
// re-direction.
func (c *connection) processMessages() {
	// Range over all messages until we encounter a control message from
	// the Middle hub.
	for msg := range c.proc {
		// Pass control message to the send chan to ensure all internal channels
		// have seen the control message.
		if msg == nil || msg.Type == controlClose {
			c.send <- msg
			return
		} else {
			// log.Println("received message on proc chan")
			// spew.Dump(msg)
			pmsg, ok, err := c.h(msg)
			if err != nil {
				log.Println("Handler error:", err)
				c.signalClose()
				continue
			}

			// Only pass messages the handler deems worthy.
			if ok {
				pmsg.Origin = c.peer // message writers can ignore the Origin.
				c.comm <- pmsg
			}
		}
	}
}

// writeMessages pumps messages from the Middle hub to the  websocket.
// It also keeps the underlying websocket connection alive by sending pings.
func (c *connection) writeMessages() {
	// ws := c.ws
	ticker := time.NewTicker(c.pingPeriod)

	defer func() {
		ticker.Stop()
		c.close()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			if !ok || msg == nil || msg.Type == controlClose {
				log.Println("Connection ", c.peer, "received control close msg")
				return
			}

			if err := c.write(msg); err != nil {
				c.signalClose()
			}

		case <-ticker.C:
			control := &Message{websocket.PingMessage, nil, internal}
			if err := c.write(control); err != nil {
				c.signalClose()
			}
		}
	}
}

func (c *connection) run() {
	go c.writeMessages()
	go c.processMessages()
	go c.readMessages()
}

func newConnection(ws *websocket.Conn, h MessageHandler, peer uint8, conf *connectionConfig) *connection {
	return &connection{
		ws:               ws,
		h:                h,
		peer:             peer,
		send:             make(chan *Message),
		read:             make(chan *Message),
		proc:             make(chan *Message),
		connectionConfig: conf,
	}
}
