// Package wemdigo provides a Middle struct that allows for
// multiple websockets to communicate with each other while a middle layer
// adds interception processing.  Moreover, the Middle layer handles
// ping & pong communications.
package wemdigo

import (
	"errors"
	"sync"
	"time"

	"github.com/tj/go-debug"
)

var dlog = debug.Debug("wemdigo")

type connSet map[*Conn]struct{}

// Middle between a collection of websockets.
type Middle struct {
	conns     connSet
	targets   map[string]*Conn // named connections
	hubs      map[string]connSet
	handler   MessageHandler
	unhandled chan *Message
	handled   chan *Message
	regConn   chan *Conn
	unreg0    chan *Conn // staging area for conns to unregister.
	unreg     chan *Conn // conns ready to be terminated.
	cmd       chan *evaluate
	done      chan struct{}

	// reg chan ConnConfig
	// FIXME do we need these?
	// regTarget chan string
	// regHub  chan string
}

func (s connSet) add(c *Conn) {
	s[c] = struct{}{}
}

func (s connSet) merge(C connSet) {
	for c := range C {
		s.add(c)
	}
}

func (s connSet) has(c *Conn) bool {
	_, ok := s[c]
	return ok
}

func (m Middle) hasTarget(key string) bool {
	c, ok := m.targets[key]
	if ok && m.conns.has(c) {
		return true
	}
	return false
}

// isHealthy indicates whether the input connection should be removed from
// the Middle's management.  Currently this will return true as long as
// all of argument's connection dependencies are open.
func (m *Middle) isHealthy(c *Conn) bool {
	for _, key := range c.Deps() {
		if !m.hasTarget(key) {
			return false
		}
	}
	return true
}

// register a fully initialized connection with the Middle instance.
// This will add the connection to the appropriate sets.
func (m *Middle) register(c *Conn) error {

	// Add c as a target if it has a key and this target key is available.
	// Otherwise, return a registration error.
	if key, ok := c.Key(); ok {
		t, prs := m.targets[key]
		if !prs {
			m.targets[key] = c
		} else {
			if t != c {
				return errors.New(ErrTargetKeyInUse)
			}
		}
	}

	// Add the connection to the set of the Middle's maintained conns.
	m.conns.add(c)

	// Subscribe the connection to its targets' hubs.
	for _, tkey := range c.Targets() {
		_, prs := m.hubs[tkey]
		if !prs {
			m.hubs[tkey] = make(connSet)
		}
		m.hubs[tkey].add(c)
	}

	// Subscribe the connection to general hubs.
	for _, tkey := range c.Hubs() {
		_, prs := m.hubs[tkey]
		if !prs {
			m.hubs[tkey] = make(connSet)
		}
		m.hubs[tkey].add(c)
	}

	return nil
}

// unregister a connection from the Middle instance.  This will
// delete the connection from the relevant sets and should only be
// called from the main logic loop.
func (m *Middle) unregister(c *Conn) error {
	if !m.conns.has(c) {
		return nil
	}

	var err error

	// Check whether the conn is a target. If so, try to delete it from
	// the set of target connections.
	if key, ok := c.Key(); ok {
		if c2, ok := m.targets[key]; ok {
			if c != c2 {
				err = errors.New(ErrTargetKeyInUse)
			} else {
				// Also delete the associated hub.
				delete(m.targets, key)
				delete(m.hubs, key)
			}
		}
	}

	// Remove the connection from its hubs.
	for _, hkey := range c.Hubs() {
		hub := m.hubs[hkey]
		delete(hub, c) // safe even on nil maps
	}

	// Remove the connection from the Middle's connection set.
	delete(m.conns, c)

	c.done <- struct{}{}
	close(c.done)
	return err
}

// During a Middle heartbeat, unregister any unhealthy connections.
func (m *Middle) unregisterUnhealthy() {
}

// destinations computes a set of Conns from the message's Destination field.
func (m *Middle) destinations(msg *Message) connSet {
	// When the message should be broadcast to everyone.
	if msg.dest.broadcast {
		return m.conns
	}

	s := make(connSet)
	// Take the union over all specified hubs.
	for _, k := range msg.dest.Hubs {
		hub := m.hubs[k]
		s.merge(hub)
	}
	// Add all specified targets.
	for _, k := range msg.dest.Targets {
		if conn, ok := m.targets[k]; ok {
			s.add(conn)
		}
	}
	// Add any special connections.
	for _, conn := range msg.dest.conns {
		s.add(conn)
	}

	return s
}

// TODO
// send the message to the specified connection.
func (m *Middle) send(msg *Message, c *Conn) {
	dlog("Sending to conn %s", c.key)
	c.wgRec.Add(1)
	go func() {
		defer c.wgRec.Done()
		c.send <- msg
	}()
}

// broadcast the message to the appropriate set of connections.
func (m *Middle) broadcast(msg *Message) {
	if msg == nil {
		return
	}

	conns := m.destinations(msg)
	for conn := range conns {
		m.send(msg, conn)
	}
}

// handlerLoop watches for raw messages sent from the Middle's connections
// and applies the message handler to each message in a separate goroutine.
// The results, and any potential errors, and sent back to the Middle through
// the appropriate channels.
func (m *Middle) handlerLoop() {
	var wg sync.WaitGroup
	for msg := range m.unhandled {
		if msg == nil {
			continue
		}

		wg.Add(1)
		msg.origin.wgSend.Add(1)
		if msg == nil {
			dlog("Message sent to unhandled chan is nil.")
		}
		go func(x *Message) {
			defer func() {
				wg.Done()
				msg.origin.wgSend.Done()
			}()

			pmsg := m.handler(x)
			if pmsg == nil {
				return
			}

			if pmsg.origin == nil {
				pmsg.origin = msg.origin
			}

			m.handled <- m.handler(x)
		}(msg)
	}

	wg.Wait()
	dlog("Closing Middle's handled channel.")
	close(m.handled)
}

func (m *Middle) shutdown() {
	for c := range m.conns {
		m.unregister(c)
	}
}

// Shutdown the Middle layer and close all underlying websockets.
func (m *Middle) Shutdown() {
	if len(m.conns) == 0 {
		return
	}
	select {
	case m.done <- struct{}{}:
	default:
		return
	}
}

func (m *Middle) mainLoop() {
	pulse := time.NewTicker(30 * time.Second)
	defer func() {
		pulse.Stop()
		close(m.unreg0)
		close(m.unreg)
		close(m.unhandled)
	}()

	for {
		// If at any point a Middle instance has no connections, begin shutdown.
		if len(m.conns) == 0 {
			dlog("No more connections remain.  Shutting down.")
			return
		}

		select {
		case <-pulse.C:
			// Check the health of each connection and unregister the sick.
			for c := range m.conns {
				if !m.isHealthy(c) {
					dlog("Conn %s is unhealthy.  Staging for unregistration.", c.key)
					go func() {
						m.unreg0 <- c
					}()
				}
			}
		case msg, ok := <-m.handled:
			// A processed (handled) message has arrived.
			if ok {
				m.broadcast(msg)
			}
		case c := <-m.regConn:
			// Register a new connection.
			m.register(c)
		case c := <-m.unreg0:
			// Staging area for connections that will soon be unregistered.
			// We attempt to let the connection write all pertinent messages
			// before unregistering.
			dlog("Conn %s is staging for unregistration.", c.key)
			go func() {
				c.wgRec.Wait()
				m.unreg <- c
			}()
		case c := <-m.unreg:
			// Unregister the connection.  Closes and websocket.
			dlog("Conn %s unregistering.", c.key)
			m.unregister(c)
		case e := <-m.cmd:
			e.eval()
		case <-m.done:
			// Stop processing messages.
			dlog("Middle is shutting down.")
			m.shutdown()
			return
		}
	}
}

// InitConn will create a new Conn instance from the input config
// that is set up appropriately for use with the middle instance.
func (m *Middle) initConn(cc ConnConfig) *Conn {
	c := NewConn()
	c.Init(cc, m.unhandled, m.unreg)
	return c
}

// FIXME: refactor to use names, etc.
// Run a Middle layer until all of it's connections have closed.
func (m Middle) Run() {
	for c := range m.conns {
		c.run()
	}

	// Apply the message handler to incoming messages.
	go m.handlerLoop()
	go m.mainLoop()
}

// SetHandler will set the Middle layer's message handler to the input.
// If the input is not specified, the package's default message handler
// is used.
func (m *Middle) SetHandler(f MessageHandler) {
	if f == nil {
		m.handler = DefaultMessageHandler
	} else {
		m.handler = f
	}
}

func (m *Middle) RegisterConn(cc ConnConfig) error {
	f := func() error {
		c := m.initConn(cc)
		return m.register(c)
	}
	e := &evaluate{f: f}
	m.cmd <- e
	return <-e.err
}

func (m *Middle) Init(cconfs []ConnConfig, h MessageHandler, wsc *WSConfig) {
	m.SetHandler(h)

	// Crete, initialize, and register each connection with the Middle.
	// This will add the Conn instances to the appropriate sets to which
	// they belong.
	for _, cc := range cconfs {
		if wsc != nil && cc.WSConf == nil {
			cc.WSConf = wsc
		}
		c := m.initConn(cc)
		m.register(c)
	}
}

// TODO add Register Service + Hub or should we assume these are static?
// func (m *Middle) RegisterConn(conf ConnConfig) {

// }
// func (m *Middle) RegisterHub(id string, conns ...ConnConfig) {

// }
// func (m *Middle) RegisterService(ws *websocket.Conn, id string, conns ...ConnConfig) {
// }

// FIXME
// func (m *Middle) AddService(ws *websocket.Conn, id string) {}
// func (m *Middle) AddHub(id string) {}
// func (m *Middle) AddTarget(ws *websocket.Conn, id string) {}

func NewMiddle() *Middle {
	return &Middle{
		conns:     make(connSet),
		targets:   make(map[string]*Conn),
		hubs:      make(map[string]connSet),
		unhandled: make(chan *Message),
		handled:   make(chan *Message),
		regConn:   make(chan *Conn),
		unreg0:    make(chan *Conn),
		unreg:     make(chan *Conn),
		done:      make(chan struct{}),
		cmd:       make(chan *evaluate),
	}
}

func New(conf Config) *Middle {
	m := NewMiddle()
	m.Init(conf.ConnConfigs, conf.Handler, conf.WSConf)
	return m
}
