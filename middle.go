// Package wemdigo provides structs that allows for bidirectional middle
// processing of websocket communications between a client and server.
package wemdigo

import (
	"errors"
	"log"
	"time"

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
	// Optional params
	// WriteWait is the time allowed to write a message to the peer.
	WriteWait time.Duration
	// PingPeriod specifies how often to ping the peer.  It determines
	// a slightly longer pong wait time.
	PingPeriod time.Duration
	// Pong Period specifies how long to wait for a pong response.  It
	// should be longer than the PingPeriod, and generally does not need
	// to be set by the user.
	PongWait time.Duration
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
		c.send <- msg
		return nil
	}

	return errors.New("Could not redirect message.")
}

// Process a websocket connection. Handles reading and writing.
func (m Middle) processConn(c *connection) {
	go c.writeMessages()
	go c.processMessages(m.messages)
	go c.readMessages()
}

func (m Middle) Shutdown() {
	for _, conn := range m.conns {
		conn.ws.Close()
		close(conn.send)
	}
	close(m.messages)
}

func (m Middle) Run() {
	for _, conn := range m.conns {
		m.processConn(conn)
	}

	// Redirect messages sent from the websockets.
	// FIXME seems equivalent to a range over the message channel.
	// defer m.Shutdown()
	// for {
	// 	select {
	// 	case msg, ok := <-m.messages:
	// 		if !ok {
	// 			m.Shutdown()
	// 		}
	// 		err := m.redirect(msg)
	// 		if err != nil {
	// 		}
	// 	}
	// }

	defer m.Shutdown()
	for msg := range m.messages {
		// spew.Dump("redirecting message in middle:", msg)
		err := m.redirect(msg)
		if err != nil {
			log.Println(err)
			return
		}
	}
}

func New(conf *Config) *Middle {

	// Inspect the optional connection stay alive config params and set
	// them if necessary.
	if conf.PingPeriod == 0 {
		conf.PingPeriod = 60 * time.Second
	}
	if conf.WriteWait == 0 {
		conf.WriteWait = 10 * time.Second
	}
	if conf.PongWait == 0 {
		conf.PongWait = (10 * conf.PingPeriod) / 9
	}

	c := newConnection(conf.ClientWebsocket, conf.ClientHandler, Client, conf)
	s := newConnection(conf.ServerWebsocket, conf.ServerHandler, Server, conf)
	return &Middle{
		conns: map[string]*connection{
			Client: c,
			Server: s,
		},
		messages: make(chan *Message),
	}
}
