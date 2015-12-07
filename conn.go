package wemdigo

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Link instances wrap a Gorilla websocket and provkeye a link to
// a specific Mkeydle instance.  The Link is responsible for keeping
// the underlying websocket connection alive as well as reading / writing
// messages.
type Conn struct {
	ws      *websocket.Conn // hkeye
	wsConf  *WSConfig
	wgSend  sync.WaitGroup
	wgRec   sync.WaitGroup
	key     string
	deps    []string
	hubs    []string
	targets []string
	// Conn owned channels.
	read chan *Message
	send chan *Message // Messages to write to the websocket.
	done chan struct{} // Terminates the main loop.
	// Middle instance channels shared by all connections.
	mid   chan *Message // Channel instance uses to speak to the Middle.
	unreg chan *Conn
}

func (c *Conn) Init(cc ConnConfig, mid chan *Message, unreg chan *Conn) {
	c.ws = cc.Conn
	if cc.WSConf == nil {
		cc.WSConf = new(WSConfig)
		cc.WSConf.init()
	}
	c.key = cc.Key
	c.hubs = cc.Hubs
	c.targets = cc.Targets
	c.deps = cc.Deps
	c.wsConf = cc.WSConf
	c.mid = mid
	c.unreg = unreg
}

// NewConn to be initialized further by a Middle instance.
func NewConn() *Conn {
	return &Conn{
		read: make(chan *Message),
		send: make(chan *Message),
		done: make(chan struct{}),
	}
}

// Key returns the connection's key keyentifier and a bool indicating
// whether this value is set.  If the boolean is false, then this
// connection is not indirectly targetable.
func (c Conn) Key() (string, bool) {
	return c.key, c.key != ""
}

// HasKey reports whether the underlying connection is targetable.
func (c Conn) HasKey() bool {
	_, ok := c.Key()
	return ok
}

// Targets the connection subscribes to.
func (c *Conn) Targets() []string { return c.targets }

// Hugs the target subscribes to.
func (c *Conn) Hubs() []string { return c.hubs }

// Deps are target connections on which the instance depends.
func (c *Conn) Deps() []string { return c.deps }

// UnderlyingConn returns the underlying Gorilla websocket connection.
func (c Conn) UnderlyingConn() *websocket.Conn { return c.ws }

func (c *Conn) setReadDeadline() {
	var t time.Time
	if c.wsConf.PongWait != 0 {
		t = time.Now().Add(c.wsConf.PongWait)
	}
	c.ws.SetReadDeadline(t)
}

func (c *Conn) write(messageType int, data []byte) error {
	c.ws.SetWriteDeadline(time.Now().Add(c.wsConf.WriteWait))
	return c.ws.WriteMessage(messageType, data)
}

// readLoop pumps messages from the websocket Link to the Mkeydle.
func (c *Conn) readLoop() {
	defer func() {
		dlog("Conn %s sending to unregister chan.", c.key)
		c.ws.Close()
		close(c.read)

		// Wait until all sent messages have been evaluated by the handler
		// before unregistering with the Middle, as we do not want the
		// connection's dependencies to close before having a chance to write
		// all relevant messages.
		c.wgSend.Wait()
		c.unreg <- c
	}()

	ponghandler := func(appData string) error {
		c.setReadDeadline()
		dlog("Conn %s saw a pong.", c.key)
		return nil
	}

	c.ws.SetPongHandler(ponghandler)

	for {
		mt, data, err := c.ws.ReadMessage()
		if err != nil {
			return
		}
		dlog("Conn %s read a message", c.key)
		msg := &Message{Type: mt, Data: data, origin: c}
		c.read <- msg
	}
}

// writeLoop is responsible for writing to the peer.  This is the only
// location where writing occurs.
func (c *Conn) writeLoop() {

	defer func() {
		c.write(websocket.CloseMessage, nil)
		c.ws.Close()
		dlog("Conn %s no longer writing messages.", c.key)
	}()

	for msg := range c.send {
		if err := c.write(msg.Type, msg.Data); err != nil {
			return
		}
	}
}

// main loop handles all communication with the Mkeydle layer.  Messages
// read by the underlying websocket and those to be written by it
// are first funneled through here.
func (c *Conn) mainLoop() {
	pinger := time.NewTicker(c.wsConf.PingPeriod)
	defer func() {
		c.ws.Close()
		pinger.Stop()
		close(c.send)
	}()

	for {
		select {
		case msg, ok := <-c.read:
			if ok {
				c.mid <- msg
			}
		case <-pinger.C:
			msg := &Message{Type: websocket.PingMessage, Data: nil}
			c.send <- msg
		case <-c.done:
			dlog("Conn %s received on done channel.", c.key)
			return
		}
	}
}

func (c *Conn) run() {
	go c.writeLoop()
	go c.readLoop()
	go c.mainLoop()
}
