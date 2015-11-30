// Package wemdigo provides a Middle struct that allows for
// multiple websockets to communicate with each other while a middle layer
// adds interception processing.  Moreover, the Middle layer handles
// ping & pong communications.
package wemdigo

import (
	"time"

	"github.com/gorilla/websocket"
	"github.com/tj/go-debug"
)

var dlog = debug.Debug("wemdigo")

// Config parameters used to create a new Middle instance.
type Config struct {
	// Required params.
	// FIXME: delete the conns and instead use groups?
	// Consider not requiring the *Conn?  We can use them internally
	// if we want, but probably shouldn't require them.
	Conns map[string]*websocket.Conn
	// Peers associates a connection id to a list of other websocket
	// connection ids that it can communicate with. A connection will
	// close if all of its peers have closed.
	Peers map[string][]string

	// Groups indicates which services
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

	// ReadLimit, if provided, will set an upper bound on message size.
	// This value is the same as in the Gorilla websocket package.
	ReadLimit int64
}

// Middle between a collection of websockets.
type Middle struct {
	Links      cmap
	conf       *Config
	raw        chan *Message
	message    chan *Message
	errors     chan error
	unregister chan *Link
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

func (m *Middle) SetHandler(f MessageHandler) {
	// TODO: if nil, use default handler which broadcasts messages
	// from one websocket to all other members of its group.
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
		dlog("Middle handle loop received a message.")
		go func(msg *Message) {
			pmsg, ok, err := m.conf.Handler(msg)
			if err != nil {
				// Non-blocking send to the Middle error chan. We only require
				// a single handler error to terminate the Middle's run.
				select {
				case m.errors <- err:
				default:
				}
				return
			}

			if ok {
				m.message <- pmsg
			}
		}(msg)
	}
}

// add the Gorilla websocket connection to the Middle instance.
// func (m *Middle) Add(ws *websocket.Conn, id string) {
// 	m.register <-
// }
func (m *Middle) addLink(ws *websocket.Conn, id string, peers []string) {
	l := &Link{
		ws:   NewConn(ws),
		id:   id,
		send: make(chan *Message),
		mid:  m,
	}

	// TODO set peers somehow
	// ps := m.conf.Peers[id]
	// for _, id := range ps {
	// 	l.peers[id] = struct{}{}
	// }

	if m.conf.ReadLimit != 0 {
		l.ws.SetReadLimit(m.conf.ReadLimit)
	}

	m.Links.set(id, l)
}

// Remove the websocket connection with the given id from the middle layer
// and close the underlying websocket connection.
func (m *Middle) remove(id string) {
	if l, ok := m.Links.get(id); ok {
		m.unregister <- l
	} else {
		dlog("Link with id = %s does not exist.", id)
	}
}

// Delete a Link from the Middle Links map.
func (m *Middle) delete(l *Link) {
	if l, ok := m.Links.get(l.id); ok {
		// This is the only time the send channel is closed.
		close(l.send)
		m.Links.delete(l.id)
	}
}

// send a the message to the connection with the specified id.
func (m *Middle) send(msg *Message, id string) {
	// Only try to re-route messages to connections the Middle controls.
	if l, ok := m.Links.get(id); ok {
		// The Link's writeLoop will read from l.send until the channel is closed,
		// so it is safe to always send messages.
		l.send <- msg
	} else {
		dlog("Cannot send message to non-existent Link with id = %s", id)
	}
}

func (m Middle) Run() {
	defer func() {
		// close(m.unregister)
		close(m.raw)
	}()

	for _, l := range m.Links.m {
		l.run()
	}

	// Apply the message handler to incoming messages.
	go m.handlerLoop()

	// Main event loop.
	for {
		dlog("In main Middle event loop.")
		// If at any point a Middle instance has no connections, begin shutdown.
		if m.Links.isEmpty() {
			dlog("No more connections remain.  Shutting down.")
			return
		}

		select {
		case msg := <-m.message:
			dlog("Broadcasting message to: %s", msg.Destinations)
			// Broadcast the processed message to destinations.
			for _, id := range msg.Destinations {
				m.send(msg, id)
			}

		case err := <-m.errors:
			if err != nil {
				dlog("Message handler error: %s", err.Error())
				// Send a kill message to all connections.
				for id := range m.Links.m {
					msg := &Message{}
					msg.SetCommand(Kill)
					m.send(msg, id)
				}
			}

		case l := <-m.unregister:
			dlog("Unregistering Link with id = %s", l.id)
			m.delete(l)
		}
	}

}

func New(conf Config) *Middle {
	// Expose certain aspects of the Middle layer to its connections.
	conf.init()

	m := &Middle{
		Links:      cmap{m: make(map[string]*Link, len(conf.Conns))},
		conf:       &conf,
		unregister: make(chan *Link),
		raw:        make(chan *Message),
		message:    make(chan *Message),
		errors:     make(chan error),
	}

	// Create new connections from the underlying websockets.
	for id, ws := range conf.Conns {
		m.addLink(ws, id, m.conf.Peers[id])
	}
	conf.Conns = nil

	return m
}
