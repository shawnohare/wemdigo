package wemdigo

import (
	"log"
	"time"

	"github.com/gorilla/websocket"
)

type connection struct {
	ws   *websocket.Conn // Underlying Gorilla websocket connection.
	id   string          // Peer identifier for the websocket.
	send chan *Message   // Messages to write to the websocket.
	mid  *Middle         // Middle instance to which the connection belongs.
}

func (c *connection) setReadDeadline() {
	var t time.Time
	if c.mid.conf.PongWait != 0 {
		t = time.Now().Add(c.mid.conf.PongWait)
	}
	c.ws.SetReadDeadline(t)
}

// readLoop pumps messages from the websocket connection to the Middle.
func (c *connection) readLoop() {
	defer func() {
		c.mid.unregister <- c
		c.ws.Close()
	}()

	ponghandler := func(appData string) error {
		c.setReadDeadline()
		log.Println("conn", c.id, "saw a pong.")
		return nil
	}

	c.ws.SetPongHandler(ponghandler)

	for {
		mt, data, err := c.ws.ReadMessage()
		if err != nil {
			return
		}
		log.Println("conn", c.id, "saw message", string(data))
		message := &Message{Type: mt, Data: data, Origin: c.id}
		c.mid.raw <- message
	}
}

func (c *connection) write(messageType int, data []byte) error {
	c.ws.SetWriteDeadline(time.Now().Add(c.mid.conf.WriteWait))
	return c.ws.WriteMessage(messageType, data)
}

// writeLoop pumps messages from the Middle to the websocket connection.
func (c *connection) writeLoop() {
	ticker := time.NewTicker(c.mid.conf.PingPeriod)
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

			// Deal with possible control messages.
			switch message.Control {
			case Kill:
				c.write(websocket.CloseMessage, nil)
				return
			default:
				// No Control value set, so no special processing required.
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
	go c.writeLoop()
	go c.readLoop()
}
