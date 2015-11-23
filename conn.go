package gosm

import (
	"log"
	"time"

	"github.com/gorilla/websocket"
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

type message struct {
	from        string
	messageType int
	payload     []byte
}

type Conn struct {
	Websocket *websocket.Conn
	Handler   MessageHandler
	name      string
	send      chan message
}

func (c *Conn) write(messageType int, payload []byte) error {
	ws := c.Websocket
	ws.SetWriteDeadline(time.Now().Add(writeWait))
	return ws.WriteMessage(messageType, payload)
}

// Process incoming messages from the websocket connection and
// send the result to a Middleware hub for redirection.
func (c *Conn) readMessages(middlewareChan chan message) {
	ws := c.Websocket

	pongHandler := func(string) error {
		// FIXME: remove logging
		log.Println("Connection", c.name, "received a pong message")
		// Reset deadline.
		ws.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	}

	defer func() {
		close(c.send)
		ws.Close()
	}()

	ws.SetReadDeadline(time.Now().Add(pongWait))
	ws.SetPongHandler(pongHandler)
	for {
		mt, raw, err := ws.ReadMessage()
		if err != nil {
			break
		}

		mt, pm, err := c.Handler(mt, raw)
		if err != nil {
			// FIXME: do what in case the handler errors?
			log.Println("Handler error:", err)
			continue
		}

		msg := message{
			from:        c.name,
			messageType: mt,
			payload:     pm,
		}

		middlewareChan <- msg
	}
}

// writeMessages pumps messages from the Middleware hub to the  websocket.
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
		case m, ok := <-c.send:
			if !ok {
				ws.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.write(m.messageType, m.payload); err != nil {
				return
			}

		case <-ticker.C:
			// FIXME: remove WriteControl?
			// deadline := time.Now().Add(pongWait)
			// if err := ws.WriteControl(websocket.PingMessage, nil, deadline) {
			// 	return
			// }
			if err := c.write(websocket.PingMessage, []byte{}); err != nil {
				return
			}
		}
	}
}

func NewConn(ws *websocket.Conn, h MessageHandler) *Conn {
	return &Conn{
		Websocket: ws,
		Handler:   h,
		send:      make(chan message),
	}
}
