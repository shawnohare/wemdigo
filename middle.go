// Package wemdigo provides a Middle struct that allows for
// multiple websockets to communicate with each other while a middle layer
// adds interception processing.  Moreover, the Middle layer handles
// ping & pong communications.
package wemdigo

import (
	"log"
	"time"

	"github.com/gorilla/websocket"
)

// Config parameters used to create a new Middle instance.
type Config struct {
	// Required params.
	Conns   map[string]*websocket.Conn
	Handler MessageHandler

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

// Middle between a collection of websockets.
type Middle struct {
	conns      map[string]*connection
	handler    MessageHandler
	raw        chan *RawMessage
	message    chan *Message
	errors     chan error
	unregister chan *connection
}

func (conf *Config) init() {
	// Process the config
	if conf.PingPeriod == 0 {
		conf.PingPeriod = 20 * time.Second
	}
	if conf.WriteWait == 0 {
		conf.WriteWait = 10 * time.Second
	}
	if conf.PongWait == 0 {
		conf.PongWait = 1 + ((10 * conf.PingPeriod) / 9)
	}
}

// handlerLoop watches for raw messages sent from the Middle's connections
// and applies the message handler to each message in a separate goroutine.
// The results, and any potential errors, and sent back to the Middle through
// the appropriate channels.
func (m Middle) handlerLoop() {
	defer func() {
		close(m.message)
		close(m.errors)
	}()

	for msg := range m.raw {
		go func(msg *RawMessage) {
			pmsg, ok, err := m.handler(msg)
			if err != nil {
				m.errors <- err
				return
			}

			if ok {
				m.message <- pmsg
			}
		}(msg)
	}
}

// Delete a connection from the Middle connections map.
func (m *Middle) delete(c *connection) {
	if _, ok := m.conns[c.id]; ok {
		close(c.send)
		delete(m.conns, c.id)
	}
}

// send a the message to the connection with the specified id.
func (m *Middle) send(msg *Message, id string) {
	if c, ok := m.conns[id]; ok {
		select {
		case c.send <- msg:
		default:
			m.delete(c)
		}
	} else {
		log.Println("Connection with id", id, "does not exist.")
	}
}

func (m Middle) Run() {
	defer func() {
		close(m.unregister)
		close(m.raw)
	}()

	log.Println("Running middle.")
	for _, conn := range m.conns {
		conn.run()
	}

	// Apply the message handler to incoming messages.
	go m.handlerLoop()

	// Main event loop.
	for {
		// If at any point a Middle instance has no connections, begin shutdown.
		if len(m.conns) == 0 {
			break
		}

		select {

		case msg := <-m.message:
			// Broadcast the processed message to destinations.
			for _, id := range msg.Destinations {
				m.send(msg, id)
			}

		case err := <-m.errors:
			if err != nil {
				log.Println("Message handler error:", err)
				// Send a kill message to all connections.
				for id := range m.conns {
					control := &Message{Control: Kill}
					m.send(control, id)
				}
			}

		case c := <-m.unregister:
			m.delete(c)
		}
	}

}

func New(conf *Config) *Middle {

	unregister := make(chan *connection)

	// Expose certain aspects of the Middle layer to its connections.
	conf.init()
	cc := &connectionConfig{
		unregister: unregister,
		writeWait:  conf.WriteWait,
		pingPeriod: conf.PingPeriod,
		pongWait:   conf.PongWait,
	}

	// Create new connections from the underlying websockets.
	conns := make(map[string]*connection, len(conf.Conns))
	for peer, ws := range conf.Conns {
		conn := newConnection(ws, peer, cc)
		conns[peer] = conn
	}

	m := &Middle{
		conns:      conns,
		handler:    conf.Handler,
		unregister: unregister,
		raw:        make(chan *RawMessage),
		message:    make(chan *Message),
		errors:     make(chan error),
	}
	return m
}
