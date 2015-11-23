// Package gosm provides structs that allows for additional
// processing of websocket communications between a client and server.
package gosm

import (
	"errors"
	"log"
)

type MessageHandler func(int, []byte) (int, []byte, error)

type Middleware struct {
	conns    map[string]*Conn
	messages chan message
	// ClientConn *Conn
	// ServerConn *Conn
	// Conns         Conns
	// ClientHandler MessageHandler
	// ServerHandler MessageHandler
}

func (m Middleware) redirect(msg message) error {

	dest := "server"
	if msg.from == "server" {
		dest = "client"
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
		messages: make(chan message),
	}
}

// func (m Middleware) Start() {
// 	go func() {
// 		m.Conns.KeepAlive()
// 	}()

// 	// Handle messages from the client to the server.
// 	go func() {
// 		intercept(m.Conns.ClientConn, m.Conns.ServerConn, m.ClientHandler)
// 	}()

// 	// Handle messages from the server to the client.
// 	go func() {
// 		intercept(m.Conns.ServerConn, m.Conns.ClientConn, m.ServerHandler)
// 	}()
// }

// // New Middleware instance with client message handler c and server
// // message handler s.
// func New(conns Conns, c MessageHandler, s MessageHandler) *Middleware {
// 	return &Middleware{conns, c, s}
// }
