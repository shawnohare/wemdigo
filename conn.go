package wemdigo

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Link instances wrap a Gorilla websocket and provide a link to
// a specific Middle instance.  The Link is responsible for keeping
// the underlying websocket connection alive as well as reading / writing
// messages.
type Conn struct {
	ws         *websocket.Conn // hide
	wg         sync.WaitGroup
	isService  bool               // For the user's convenience.
	id         string             // Optional string identifier, if instance is a target.
	deps       map[*Conn]struct{} // Conns on which the instance depends.
	read       chan *Message
	send       chan *Message // Messages to write to the websocket.
	message    chan *Message // Channel the Middle uses to speak to the Conn.
	unregister chan struct{}
	done       chan struct{} // Terminates the main loop.
	mid        *Middle       // Middle instance to which the Link belongs.
}

// IsService reports whether the connection corresponds to some service.
func (c Conn) IsService() bool {
	return c.isService
}

// UnderlyingConn returns the underlying Gorilla websocket connection.
func (c Conn) UnderlyingConn() *websocket.Conn {
	return c.ws
}

// ID associated to this connection.
func (c Conn) ID() string {
	return c.id
}

func (c *Conn) setReadDeadline() {
	var t time.Time
	if c.mid.conf.pongWait != 0 {
		t = time.Now().Add(c.mid.conf.pongWait)
	}
	c.ws.SetReadDeadline(t)
}

func (c *Conn) write(messageType int, data []byte) error {
	c.ws.SetWriteDeadline(time.Now().Add(c.mid.conf.writeWait))
	return c.ws.WriteMessage(messageType, data)
}

// readLoop pumps messages from the websocket Link to the Middle.
func (c *Conn) readLoop() {
	defer func() {
		dlog("Conn %s sending to unregister chan.", c.id)
		c.unregister <- struct{}{}
		c.ws.Close()
		close(c.unregister)
		close(c.read)
	}()

	ponghandler := func(appData string) error {
		c.setReadDeadline()
		dlog("Conn %s saw a pong.", c.id)
		return nil
	}

	c.ws.SetPongHandler(ponghandler)

	for {
		mt, data, err := c.ws.ReadMessage()
		if err != nil {
			return
		}
		dlog("Conn %s read a message", c.id)
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
		dlog("Conn %s no longer writing messages.", c.id)
	}()

	for msg := range c.send {
		if err := c.write(msg.Type, msg.Data); err != nil {
			return
		}
	}
}

// main loop handles all communication with the Middle layer.  Messages
// read by the underlying websocket and those to be written by it
// are first funneled through here.
func (c *Conn) mainLoop() {
	pinger := time.NewTicker(c.mid.conf.pingPeriod)
	defer func() {
		c.ws.Close()
		pinger.Stop()
		close(c.send)
	}()

	for {
		select {
		case msg := <-c.read:
			c.mid.raw <- msg
		case msg := <-c.message:
			c.send <- msg
		case <-pinger.C:
			msg := &Message{Type: websocket.PingMessage, Data: nil}
			c.send <- msg
		case <-c.done:
			dlog("Conn %s received on done channel.", c.id)
			return
		}
	}
}

func (c *Conn) run() {
	go c.writeLoop()
	go c.readLoop()
	go c.mainLoop()
}
