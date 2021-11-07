package signalr

import (
	"context"
	"sync"
	"time"
)

// ConnectionBase is a baseclass for implementers of the Connection interface.
type ConnectionBase struct {
	mx           sync.RWMutex
	ctx          context.Context
	connectionID string
}

// NewConnectionBase creates a new ConnectionBase
func NewConnectionBase(ctx context.Context, connectionID string) *ConnectionBase {
	cb := &ConnectionBase{
		ctx:          ctx,
		connectionID: connectionID,
	}
	return cb
}

// Context can be used to wait for cancellation of the Connection
func (cb *ConnectionBase) Context() context.Context {
	cb.mx.RLock()
	defer cb.mx.RUnlock()
	return cb.ctx
}

// ConnectionID is the ID of the connection.
func (cb *ConnectionBase) ConnectionID() string {
	cb.mx.RLock()
	defer cb.mx.RUnlock()
	return cb.connectionID
}

// SetConnectionID sets the ConnectionID
func (cb *ConnectionBase) SetConnectionID(id string) {
	cb.mx.Lock()
	defer cb.mx.Unlock()
	cb.connectionID = id
}

// Timeout is the timeout of the Connection
func (cb *ConnectionBase) Timeout() time.Duration {
	cb.mx.RLock()
	defer cb.mx.RUnlock()
	return cb.timeout
}

// SetTimeout sets the Timeout
func (cb *ConnectionBase) SetTimeout(duration time.Duration) {
	cb.mx.Lock()
	defer cb.mx.Unlock()
	cb.timeout = duration
}
