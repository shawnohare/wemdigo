package wemdigo

import (
	"log"
	"time"

	"github.com/gorilla/websocket"
)

// connectConfig are params that is typically shared amongst
// multiple connection instances.  It ties the connection to a particular
// Middle layer.
type connectionConfig struct {
	unregister chan *connection // To notify the middle the connection is done.
	raw        chan *RawMessage
	pingPeriod time.Duration
	pongWait   time.Duration
	writeWait  time.Duration
}

type connection struct {
	ws   *websocket.Conn   // Underlying Gorilla websocket connection.
	id   string            // Peer identifier for the websocket.
	send chan *Message     // Messages to write to the websocket.
	mid  *connectionConfig // General configs inhereited from Middle.
}

// readPump pumps messages from the websocket connection to the Middle.
func (c *connection) readPump() {
	defer func() {
		c.mid.unregister <- c
		c.ws.Close()
	}()

	ponghandler := func(appData string) error {
		c.ws.SetReadDeadline(time.Now().Add(c.mid.pongWait))
		log.Println("conn", c.id, "received a pong.")
		return nil
	}
	// c.ws.SetReadLimit(maxMessageSize)
	c.ws.SetReadDeadline(time.Now().Add(c.mid.pongWait))
	c.ws.SetPongHandler(ponghandler)

	for {
		mt, data, err := c.ws.ReadMessage()
		if err != nil {
			return
		}
		message := &RawMessage{Type: mt, Data: data, Origin: c.id}
		c.mid.raw <- message
	}
}

func (c *connection) write(messageType int, data []byte) error {
	c.ws.SetWriteDeadline(time.Now().Add(c.mid.writeWait))
	return c.ws.WriteMessage(messageType, data)
}

// writePump pumps messages from the Middle to the websocket connection.
func (c *connection) writePump() {
	ticker := time.NewTicker(c.mid.pingPeriod)
	defer func() {
		ticker.Stop()
		c.ws.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				c.write(websocket.CloseMessage, nil)
				return
			}

			// Handle regular messages control messages.
			switch message.Control {
			case Kill:
				c.write(websocket.CloseMessage, nil)
				return
			default:
				if err := c.write(message.Type, message.Data); err != nil {
					return
				}
			}

		case <-ticker.C:
			if err := c.write(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *connection) run() {
	go c.writePump()
	go c.readPump()
}

func newConnection(ws *websocket.Conn, id string, conf *connectionConfig) *connection {
	return &connection{
		ws:   ws,
		id:   id,
		send: make(chan *Message),
		mid:  conf,
	}
}
