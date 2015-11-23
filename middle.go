// Package wemdigo provides structs that allows for bidirectional middle
// processing of websocket communications between a client and server.
package wemdigo

import (
	"errors"
	"log"
)

// MessageHandler funcs are responsible for processing websocket messages.
// They should return a processed message, an indication of whether the
// message should be forwarded, and a possible error.  When a Middle
// instance encounters a handler error, it will shut down the underlying
// websocket.
type MessageHandler func(*Message) (*Message, bool, error)

// Middle between a client and server that would normally connect via
// a single websocket.
type Middle struct {
	conns    map[string]*Conn
	messages chan *Message
}

func (m Middle) redirect(msg *Message) error {

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
func (m Middle) processConn(c *Conn) {
	go c.writeMessages()
	c.readMessages(m.messages)
}

func (m Middle) Shutdown() {
	for _, conn := range m.conns {
		conn.Websocket.Close()
		close(conn.send)
	}
	close(m.messages)
}

func (m Middle) Run() {
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

func New(clientConn *Conn, serverConn *Conn) *Middle {
	return &Middle{
		conns:    map[string]*Conn{"client": clientConn, "server": serverConn},
		messages: make(chan *Message),
	}
}
