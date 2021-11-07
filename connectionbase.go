package signalr

import (
	"context"
	"sync"
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

func ReadWriteWithContext(ctx context.Context, doRW func() (int, error), unblockRW func()) (int, error) {
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}
	resultChan := make(chan RWJobResult, 1)
	go func() {
		n, err := doRW()
		resultChan <- RWJobResult{n: n, err: err}
		close(resultChan)
	}()
	select {
	case <-ctx.Done():
		unblockRW()
		return 0, ctx.Err()
	case r := <-resultChan:
		return r.n, r.err
	}
}

type RWJobResult struct {
	n   int
	err error
}
