package wemdigo

import (
	"encoding/json"
	"log"

	"github.com/gorilla/websocket"
)

// Conn is a Gorilla websocket Conn that can also read / write
// wemdigo Message types.
type Conn struct {
	*websocket.Conn
}

// Write will attempt to JSON encode a Message instance and write this
// to the peer.  If the Message cannot be encoded into JSON, its raw
// data payload is written instead.
func (c *Conn) Write(msg *Message) error {
	// Encode the messgae payload.
	msg.Meta.Encoded = true
	payload, err := json.Marshal(msg)
	if err != nil {
		log.Println("[wemdigo] Could not marshal Message.")
		payload = msg.Data
	}
	return c.WriteMessage(msg.Type, payload)
}

func (c *Conn) WriteCommand(cmd int, dests []string) error {
	msg := &Message{Type: websocket.BinaryMessage, Destinations: dests}
	err := msg.SetCommand(cmd)
	if err != nil {
		return err
	}
	return c.Write(msg)
}

// Read a Message from the underlying websocket.  This extends the built-in
// Gorilla websocket read commands to better handle wemdigo Messages.
// If the raw payload unmarshals into a Message instance already,
// this is returned.  Otherwise, the raw data payload and message type
// are wrapped in a Message instance and returned.
func (c *Conn) Read() (*Message, error) {
	mt, raw, err := c.ReadMessage()
	if err != nil {
		return nil, err
	}

	// FIXME
	log.Println("[wemdigo] Raw payload from peer:", string(raw))
	// First try to decode the raw bytes into a Message instance.
	msg := Message{}
	err = json.Unmarshal(raw, &msg)
	if err == nil && msg.IsEncoded() {
		log.Println("[wemdigo] Decoded message.")
		return &msg, nil
	}

	// Otherwise, wrap the raw payload in a Message instance.
	log.Println("[wemdigo] Could not decode into a Message type.")
	msg.Type = mt
	msg.Data = raw
	return &msg, nil
}

func NewConn(ws *websocket.Conn) *Conn {
	return &Conn{Conn: ws}
}
