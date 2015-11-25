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
	controlClose     int = -1
	controlKillConns     = -2
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
	conns    map[uint8]*connection
	killPool map[*connection]struct{}
	// comm is a channel over which the websockets can communicate
	comm chan *Message
	// kill allows connections to signify they are ready to be closed.
	kill chan *connection
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

// Shutdown sends a close control message to each connection so that it can
// finish the current processing & writing and begin to close.
func (m Middle) Shutdown() {
	// FIXME:
	log.Println("In Middle shutdown.")
	for _, conn := range m.conns {
		control := &Message{controlClose, nil, internal}
		conn.read <- control
	}

	// Wait for all connections to signify that they are ready to kill.
	// Then, commence cleanup operations.
	log.Println("Beginning to range over kill chan")
	for conn := range m.kill {
		log.Println("Adding conn", conn.peer, "to kill pool.")
		m.killPool[conn] = struct{}{}
		if len(m.killPool) != len(m.conns) {
			continue
		} else {
			break
		}
	}

	// All the connections in m.conns have been added to the kill pool now.
	// We can go ahead and safely close things.
	log.Println("Closing connections.")
	for conn := range m.killPool {
		conn.close()
	}

	close(m.comm)
	close(m.kill)
	log.Println("Finished shutdown.")
}

func (m Middle) Run() {
	// FIXME
	log.Println("Running middle.")
	// defer m.Shutdown()
	for _, conn := range m.conns {
		conn.run()
	}

	// for msg := range m.comm {
	// 	log.Println("Middle received message from", msg.Origin, "of type", msg.Type)
	// 	if msg == nil || msg.Type == controlKillConns {
	// 		return
	// 	}
	// 	err := m.redirect(msg)
	// 	if err != nil {
	// 		log.Println(err)
	// 		return
	// 	}
	// }

	// FIXME an attempt to signal stopping via a stop chan
	stop := make(chan bool)
	go func() {
		for msg := range m.comm {
			log.Println("Middle received message from", msg.Origin, "of type", msg.Type)

			if msg == nil || msg.Type == controlKillConns {
				stop <- true
				return
			}

			err := m.redirect(msg)
			if err != nil {
				log.Println("redir error", err)
				stop <- true
				return
			}
		}
	}()

	<-stop
	m.Shutdown()

	// FIXME: didn't seem to help
	// go func() {
	// 	for msg := range m.comm {
	// 		spew.Dump("Middle received message: %s", msg)
	// 		err := m.redirect(msg)
	// 		if err != nil {
	// 			log.Println(err)
	// 			break
	// 		}
	// 	}

	// 	// FIXME: never executed it seems.
	// 	// Send termination signal.
	// 	log.Println("Middle sending it's own done message.")
	// 	m.done <- true
	// 	close(m.done)
	// }()

	// FIXME: this seems to never be entered.  Does it need to be closed?
	// for shutdown := range m.done {
	// 	if shutdown {
	// 		log.Println("Middle received shutdown message.")
	// 		break
	// 	}
	// }

	// FIXME original middle loop that seemed to never receive things on the done chan.
	// for {
	// 	select {
	// 	case msg, ok := <-m.comm:
	// 		// FIXME
	// 		spew.Dump("Middle received message: %s", msg)
	// 		if !ok {
	// 			return
	// 		}
	// 		err := m.redirect(msg)
	// 		if err != nil {
	// 			log.Println(err)
	// 			return
	// 		}
	// 	case shutdown, ok := <-m.done:
	// 		// FIXME
	// 		log.Println("Middle received shutdown message.")
	// 		if !ok || shutdown {
	// 			return
	// 		}
	// 	}
	// }

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
	kill := make(chan *connection)

	// Expose certain aspects of the Middle layer to its connections.
	cc := &connectionConfig{
		comm:       comm,
		kill:       kill,
		writeWait:  conf.WriteWait,
		pingPeriod: conf.PingPeriod,
		pongWait:   conf.PongWait,
	}

	// Create the client and server connection.
	c := newConnection(conf.ClientWebsocket, conf.ClientHandler, Client, cc)
	s := newConnection(conf.ServerWebsocket, conf.ServerHandler, Server, cc)
	conns := map[uint8]*connection{
		Client: c,
		Server: s,
	}

	return &Middle{
		conns: conns,
		comm:  comm,
		kill:  kill,
		// FIXME can get rid of stop?
		killPool: make(map[*connection]struct{}, len(conns)),
	}
}
