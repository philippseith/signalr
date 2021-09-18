package signalr

import (
	"context"
	"time"
)

// ConnectionBase is a baseclass for implementers of the Connection interface.
type ConnectionBase struct {
	ctx          context.Context
	connectionID string
	timeout      time.Duration
}

// Context can be used to wait for cancellation of the Connection
func (cb *ConnectionBase) Context() context.Context {
	return cb.ctx
}

// ConnectionID is the ID of the connection.
func (cb *ConnectionBase) ConnectionID() string {
	return cb.connectionID
}

// SetConnectionID sets the ConnectionID
func (cb *ConnectionBase) SetConnectionID(id string) {
	cb.connectionID = id
}

// Timeout is the timeout of the Connection
func (cb *ConnectionBase) Timeout() time.Duration {
	return cb.timeout
}

// SetTimeout sets the Timeout
func (cb *ConnectionBase) SetTimeout(duration time.Duration) {
	cb.timeout = duration
}
