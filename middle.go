// Package wemdigo provides structs that allows for bidirectional middle
// processing of websocket communications between a client and server.
// The wemdigo Middle layer expects that all peers can respond to control
// messages.  In particular, a Middle instance expects regular pong
// responses after it sends a ping.
package wemdigo

import (
	"errors"
	"log"
	"time"

	"github.com/gorilla/websocket"
)

// Origin & peer constants.
const (
	internal       = 0
	Server   uint8 = 1
	Client   uint8 = 2
)

const (
	controlClose int = -1
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
	Origin uint8
}

// MessageHandler funcs are responsible for processing websocket messages.
// They should return a processed message, an indication of whether the
// message should be forwarded, and a possible error.  When a Middle
// instance encounters a handler error, it will shut down the underlying
// websocket.
type MessageHandler func(*Message) (*Message, bool, error)

// Config parameters used to create a new Middle instance.
type Config struct {
	// Required params.
	ClientWebsocket *websocket.Conn
	ClientHandler   MessageHandler
	ServerWebsocket *websocket.Conn
	ServerHandler   MessageHandler

	// Optional params.

	// PingPeriod specifies how often to ping the peer.  It determines
	// a slightly longer pong wait time.
	PingPeriod time.Duration

	// Pong Period specifies how long to wait for a pong response.  It
	// should be longer than the PingPeriod.  If set to the default value,
	// a new Middle layer will calculate PongWait from the PingPeriod.
	// As such, this param does not usually need to be set.
	PongWait time.Duration

	// WriteWait is the time allowed to write a message to the peer.
	// This does not usually need to be set by the user.
	WriteWait time.Duration
}

func (conf *Config) init() {
	// Process the config
	if conf.PingPeriod == 0 {
		conf.PingPeriod = 60 * time.Second
	}
	if conf.WriteWait == 0 {
		conf.WriteWait = 10 * time.Second
	}
	if conf.PongWait == 0 {
		conf.PongWait = 1 + ((10 * conf.PingPeriod) / 9)
	}
}

// Middle between a client and server that would normally connect via
// a single websocket.
type Middle struct {
	conns map[uint8]*connection
	// comm is a channel over which the websockets can communicate
	comm chan *Message
	// done is a channel that
	// FIXME can probably get rid of the done channel and only use the comm
	done chan bool
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

// FIXME
// Shutdown sends a close control message to each connection so that it can
// finish the current processing & writing and begin to close.
func (m Middle) Shutdown() {
	for _, conn := range m.conns {
		control := &Message{controlClose, nil, internal}
		conn.read <- control
	}
	// FIXME do we need to close these channels?
	// close(m.comm)
	// close(m.done)
}

func (m Middle) Run() {
	defer m.Shutdown()
	for _, conn := range m.conns {
		conn.run()
	}

	for {
		select {
		case msg, ok := <-m.comm:
			if !ok {
				return
			}
			err := m.redirect(msg)
			if err != nil {
				log.Println(err)
				return
			}
		case shutdown, ok := <-m.done:
			if !ok || shutdown {
				return
			}
		}
	}

	// FIXME old code without shutdown channel
	// defer m.Shutdown()
	// for msg := range m.comm {
	// 	// spew.Dump("redirecting message in middle:", msg)
	// 	if msg == nil || msg.Type == controlClose {
	// 		return
	// 	}

	// 	err := m.redirect(msg)
	// 	if err != nil {
	// 		log.Println(err)
	// 		return
	// 	}
	// }
}

func New(conf *Config) *Middle {
	conf.init()

	// Create the various Middle to connection communication channels.
	comm := make(chan *Message)

	// Expose certain aspects of the Middle layer to its connections.
	cc := &connectionConfig{
		comm:       comm,
		writeWait:  conf.WriteWait,
		pingPeriod: conf.PingPeriod,
		pongWait:   conf.PongWait,
	}

	// Create the client and server connection.
	c := newConnection(conf.ClientWebsocket, conf.ClientHandler, Client, cc)
	s := newConnection(conf.ServerWebsocket, conf.ServerHandler, Server, cc)

	return &Middle{
		conns: map[uint8]*connection{
			Client: c,
			Server: s,
		},
		comm: comm,
	}
}
