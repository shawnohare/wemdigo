package wemdigo

import (
	"time"

	"github.com/gorilla/websocket"
)

// Configuration for a Middle instance.
type Config struct {
	ConnConfigs []ConnConfig
	Handler     MessageHandler
	WSConf      *WSConfig
}

// ConnConfig is the user-defined configuration for a Conn instance.
type ConnConfig struct {
	Conn *websocket.Conn // Underlying Gorilla websocket.

	// Key, if set, adds the connection to the set of targets maintined
	// by the middle.  Messages can be directly sent to target connections.
	// These might represent service connections or special clients
	// that one wishes to easily reference.
	// If a connection is not a target, it is generally quite difficult to
	// redirect any messages to it, unless it is the origin of a message.
	Key string

	// Hubs  are collections of websockets and serve as an easy way for the
	// user to broadcast messages to multiple connections.
	// Each target connection has a (possibly empty) hub with the same key.
	// However, the set of hub keys is generally a superset of the target keys.
	Hubs []string
	// Named target connections the instance subscribes to.
	Targets []string

	// Named connections that must all be active for the connection to persist.
	Deps []string

	// Optional websocket configuration to use for this particular connection.
	// If not provided, a default will be initialized.
	WSConf *WSConfig
}

// WSConfig specifies a websocket configuration.
type WSConfig struct {
	// PingPeriod specifies how often to ping the peer.  It determines
	// a slightly longer pong wait time.
	PingPeriod time.Duration

	// Pong Period specifies how long to wait for a pong response.  It
	// should be longer than the PingPeriod.  If set to the default value,
	// a new Middle layer will calculate PongWait from the PingPeriod.
	// As such, this parameter does not usually need to be set.
	PongWait time.Duration

	// WriteWait is the time allowed to write a message to the peer.
	// This does not usually need to be set by the user.
	WriteWait time.Duration

	// ReadLimit, if provided, will set an upper bound on message size.
	// This value is the same as in the Gorilla websocket package.
	ReadLimit int64
}

func (conf *WSConfig) init() {
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
