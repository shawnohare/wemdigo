package wemdigo

import (
	"log"
	"time"

	"github.com/gorilla/websocket"
)

const (
	Server = "Server"
	Client = "Client"
)

// Some internal constants taken from the Gorilla websocket chat example.
const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second
	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second
	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10
)

type Message struct {
	Type   int
	Data   []byte
	Origin string
}

type Conn struct {
	Websocket *websocket.Conn
	Handler   MessageHandler
	name      string
	send      chan *Message
}

func (c *Conn) write(msg *Message) error {
	ws := c.Websocket
	ws.SetWriteDeadline(time.Now().Add(writeWait))
	return ws.WriteMessage(msg.Type, msg.Data)
}

// Process incoming messages from the websocket connection and
// send the result to a Middle hub for redirection.
func (c *Conn) readMessages(middlewareChan chan *Message) {
	ws := c.Websocket

	pongHandler := func(string) error {
		// FIXME: remove logging
		log.Println("Connection", c.name, "received a pong message")
		// Reset deadline.
		ws.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	}

	defer func() {
		ws.WriteMessage(websocket.CloseMessage, []byte{})
		ws.Close()
	}()

	ws.SetReadDeadline(time.Now().Add(pongWait))
	ws.SetPongHandler(pongHandler)
	for {
		mt, raw, err := ws.ReadMessage()
		if err != nil {
			break
		}

		msg, pass, err := c.Handler(&Message{mt, raw, c.name})

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

		middlewareChan <- msg
	}
}

// writeMessages pumps messages from the Middle hub to the  websocket.
// It also keeps the underlying websocket connection alive by sending pings.
func (c Conn) writeMessages() {
	ws := c.Websocket
	ticker := time.NewTicker(pingPeriod)

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

func NewConn(ws *websocket.Conn, h MessageHandler) *Conn {
	return &Conn{
		Websocket: ws,
		Handler:   h,
		send:      make(chan *Message),
	}
}
