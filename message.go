package wemdigo

// Control constants.
const (
	Kill int = -1
)

// Message from a websocket intended to be processed and sent to other
// websockets.
type Message struct {
	// Type is any valid value from the Gorilla websocket package.
	Type int
	// Data is the raw data as received from the websocket connection.
	Data []byte
	// Origin is the source of the message, and usually does not need to be
	// set by users when creating message handlers.  The Middle hub will
	// track the origin.
	Origin string
	// Destinations indicates the intended target websockets.
	Destinations []string
	// Control messages override some standard behavior. This is value
	// is provided for users to incorporate
	Control int
}

// MessageHandler funcs are responsible for processing websocket messages.
// They should return a processed message, an indication of whether the
// message should be forwarded, and a possible error.  An error will
// cause the Middle layer to commence shutdown operations.
//
//
type MessageHandler func(*Message) (*Message, bool, error)
