package wemdigo

// Message from a websocket intended to be processed and sent to other
// websockets.
type Message struct {
	// Type is any valid value from the Gorilla websocket package.
	Type int
	// Data is the raw data as received from the websocket connection.
	Data []byte

	origin *Conn
	dest   *Destination
	// Destinations indicates the intended target websockets.
}

// Destinations provides finer control message redirection.  For many
// use cases these do not need to be specified by the user.
type Destination struct {
	Targets []string
	Hubs    []string
	// Conns    []*Conn
	broadcast bool    // broadcast to everyone
	conns     []*Conn // send to a particular connection, i.e., for responses.
}

func (msg Message) OriginKey() string {
	key, _ := msg.origin.Key()
	return key
}

// Broadcast creates a response message which will be broadcast to all
// websocket connections in the same Middle instance as the original sender.
func (msg Message) Broadcast(messageType int, data []byte) *Message {
	return &Message{
		Type:   messageType,
		Data:   data,
		origin: msg.origin,
		dest:   &Destination{broadcast: true},
	}
}

// Response message to return to the sender.  This convenience method should be
// invoked by a handler that detects the original message does not
// need to be further processed by another service.
//
// If a handler returns the resulting response method, it will be
// be sent to the original message's websocket connection.
func (msg Message) Response(messageType int, data []byte) *Message {
	return &Message{
		Type:   messageType,
		Data:   data,
		origin: msg.origin,
		dest:   &Destination{conns: []*Conn{msg.origin}},
	}
}

// ToTargets forwards the response message to all fo the targets the sender
// subscribes to.  This is useful for client-type connections to forward
// messages to service connections for remote processing.
func (msg *Message) ToTargets(messageType int, data []byte) *Message {
	return &Message{
		Type:   messageType,
		Data:   data,
		origin: msg.origin,
		dest:   &Destination{Targets: msg.origin.Targets()},
	}
}

// ToSubscribers message result broadcast to all members that subscribe to the sender.
// This is useful when sending a server-side response to all subscribing
// clients.  If a mesage sender does not have a target key, then this message
// will be ignored.
func (msg *Message) ToSubscribers(messageType int, data []byte) *Message {
	// Set the origin's key as the only hub destination.
	var hubs []string
	if originKey, ok := msg.origin.Key(); ok {
		hubs = []string{originKey}
	}
	return &Message{
		Type:   messageType,
		Data:   data,
		origin: msg.origin,
		dest:   &Destination{Hubs: hubs},
	}
}

// ToHubs broadcasts a message to all hubs to which the sender subscribes.
func (msg *Message) ToHubs(messageType int, data []byte) *Message {
	return &Message{
		Type:   messageType,
		Data:   data,
		origin: msg.origin,
		dest:   &Destination{Hubs: msg.origin.Hubs()},
	}
}

// MessageHandler
// They are responsible for processing websocket messages.
// They should return a processed message, an indication of whether the
// message should be forwarded, and a possible error.  An error will
// cause the Middle layer to commence shutdown operations.
type MessageHandler func(*Message) *Message

// send a message to all the target's to which the sender subscribes.
func DefaultMessageHandler(msg *Message) *Message {
	if msg == nil {
		return nil
	}
	return msg.ToTargets(msg.Type, msg.Data)
}
