package wemdigo

import "fmt"

// Command constants.
const (
	// Kill (close) all destination connections.
	Kill int = 1
	// JSON encode the entire Message struct for transmit, rather than just
	// its data payload.
	EncodeJSON = 2
)

type meta struct {
	Command int  `json:"wemdigo_command,omitempty"`
	Encoded bool `json:"wemdigo_encoded,omitempty"`
	Decoded bool `json:"wemdigo_decoded,omitempty"`
}

// Message from a websocket intended to be processed and sent to other
// websockets.
type Message struct {
	// Type is any valid value from the Gorilla websocket package.
	Type int `json:"type"`
	// Data is the raw data as received from the websocket connection.
	Data []byte `json:"data"`
	// Origin is the source of the message, and usually does not need to be
	// set by users when creating message handlers.  The Middle hub will
	// track the origin.
	Origin string `json:"origin,omitempty"`
	// Destinations indicates the intended target websockets.
	Destinations []string `json:"destinations,omitempty"`
	Meta         meta     `json:"wemdigo_meta,omitempty"`
}

// SetControl sets the control code for the message, as long as
// the input is a valid control constant from this package.
func (m *Message) SetCommand(code int) error {
	switch code {
	case Kill:
	case EncodeJSON:
	default:
		return fmt.Errorf("%d is an invalid Wemdigo control code.", code)
	}
	m.Meta.Command = code
	return nil
}

// Control int constant assigned to the message. A default value
// indicates a lack of a control code.
func (m Message) Command() int {
	// Use the setter to validate the current value.
	m.SetCommand(m.Meta.Command)
	return m.Meta.Command
}

// IsDecoded reports whether this message struct was the result of a
// JSON unmarshaling from the raw websocket data payload.
func (m Message) IsDecoded() bool {
	return m.Meta.Decoded
}

// IsEncoded reports whether this message struct was marshaled into
// a JSON encoding and sent as the raw websocket data payload.
// A successful Conn.Write of a message will result in this function
// returning true.
func (m Message) IsEncoded() bool {
	return m.Meta.Encoded
}

// MessageHandler funcs map raw WebSocket messages bound in a Message
// instance to an honest Message instance.
//
// They are responsible for processing websocket messages.
// They should return a processed message, an indication of whether the
// message should be forwarded, and a possible error.  An error will
// cause the Middle layer to commence shutdown operations.
type MessageHandler func(*Message) (*Message, bool, error)
