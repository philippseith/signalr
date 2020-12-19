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

func (cb *ConnectionBase) Context() context.Context {
	return cb.ctx
}

func (cb *ConnectionBase) ConnectionID() string {
	return cb.connectionID
}

func (cb *ConnectionBase) SetConnectionID(id string) {
	cb.connectionID = id
}

func (cb *ConnectionBase) Timeout() time.Duration {
	return cb.timeout
}

func (cb *ConnectionBase) SetTimeout(duration time.Duration) {
	cb.timeout = duration
}
