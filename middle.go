// Package wemdigo provides structs that allows for bidirectional middle
// processing of websocket communications between a client and server.
package wemdigo

import (
	"errors"
	"log"

	"github.com/gorilla/websocket"
)

// Origin constants.
const (
	Server = "Server"
	Client = "Client"
)

// Message sent between a client and server over a websocket.  The
// Type and Data fields are the same as returned by the
// Gorilla websocket package.  The Origin indicates whether the
// message arrived from the client or server, and can safely be
// ignored by users unless they wish to write a single message handler
// and need to split logic based on origin.
type Message struct {
	Type   int
	Data   []byte
	Origin string
}

// MessageHandler funcs are responsible for processing websocket messages.
// They should return a processed message, an indication of whether the
// message should be forwarded, and a possible error.  When a Middle
// instance encounters a handler error, it will shut down the underlying
// websocket.
type MessageHandler func(*Message) (*Message, bool, error)

// Config parameters used to create a new Middle instance.
type Config struct {
	ClientWebsocket *websocket.Conn
	ClientHandler   MessageHandler
	ServerWebsocket *websocket.Conn
	ServerHandler   MessageHandler
}

// Middle between a client and server that would normally connect via
// a single websocket.
type Middle struct {
	conns    map[string]*connection
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

// Process a websocket connection. Handles reading and writing.
func (m Middle) processConn(c *connection) {
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

func New(conf *Config) *Middle {
	return &Middle{
		conns: map[string]*connection{
			Client: newConnection(conf.ClientWebsocket, conf.ClientHandler),
			Server: newConnection(conf.ServerWebsocket, conf.ServerHandler),
		},
		messages: make(chan *Message),
	}
}
