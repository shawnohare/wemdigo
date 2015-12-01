package wemdigo

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
	Origin *Link
	// Destinations indicates the intended target websockets.
	destinations []string
}

// SetDestinations allows a user to specify by string ID the
// message recipiants.  If this is not set, the message will be broadcast
// to all websockets that are peers of the message's origin.
func (msg *Message) SetDestinations(ds ...string) {
	msg.destinations = ds
}

func NewMessage(messageType int, data []byte, origin *Link) *Message {
	return &Message{
		Type:   messageType,
		Data:   data,
		Origin: origin,
	}
}

// MessageHandler funcs map raw WebSocket messages bound in a Message
// instance to an honest Message instance.
//
// They are responsible for processing websocket messages.
// They should return a processed message, an indication of whether the
// message should be forwarded, and a possible error.  An error will
// cause the Middle layer to commence shutdown operations.
type MessageHandler func(*Message) (*Message, bool, error)

func defaultHandler(msg *Message) (*Message, bool, error) {
	link := msg.Origin
	dests := link.Peers()
	msg.SetDestinations(dests...)
	return msg, true, nil
}
