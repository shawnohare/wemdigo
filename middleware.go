// Package gosm provides structs that allows for additional
// processing of websocket communications between a client and server.
package gosm

import (
	"errors"
	"log"
)

// MessageHandler funcs are responsible for processing websocket messages.
// They should return a processed message, an indication of whether the
// message should be forwarded, and a possible error.  When a Middleware
// instance encounters a handler error, it will shut down the underlying
// websocket.
type MessageHandler func(*Message) (*Message, bool, error)

// Middleware between a client and server that would normally connect via
// a single websocket.
type Middleware struct {
	conns    map[string]*Conn
	messages chan *Message
}

func (m Middleware) redirect(msg *Message) error {

	dest := Server
	if msg.Origin == Server {
		dest = Client
	}

	if c, ok := m.conns[dest]; ok {
		select {
		case c.send <- msg:
			return nil
		default:
			close(c.send)
		}
	}

	return errors.New("Could not redirect message.")
}

// Process a websocket connection.
func (m Middleware) processConn(c *Conn) {
	go c.writeMessages()
	c.readMessages(m.messages)
}

func (m Middleware) Shutdown() {
	for _, conn := range m.conns {
		conn.Websocket.Close()
		close(conn.send)
	}
	close(m.messages)
}

func (m Middleware) Run() {
	for _, conn := range m.conns {
		go m.processConn(conn)
	}

	// Redirect messages sent from the websockets.
	for msg := range m.messages {
		err := m.redirect(msg)
		if err != nil {
			log.Println(err)
			m.Shutdown()
		}
	}
}

func New(clientConn *Conn, serverConn *Conn) *Middleware {
	return &Middleware{
		conns:    map[string]*Conn{"client": clientConn, "server": serverConn},
		messages: make(chan *Message),
	}
}
