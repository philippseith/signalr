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

// ReadWriteWithContext is a wrapper to make blocking io.Writer / io.Reader cancelable.
// It can be used to implement cancellation of connections.
// ReadWriteWithContext will return when either the Read/Write operation has ended or ctx has been canceled.
//  doRW func() (int, error)
// doRW should contain the Read/Write operation.
//  unblockRW func()
// unblockRW should contain the operation to unblock the Read/Write operation.
// If there is no way to unblock the operation, one goroutine will leak when ctx is canceled.
// As the standard use case when ReadWriteWithContext is canceled is the cancellation of a connection this leak
// will be problematic on heavily used servers with uncommon connection types. Luckily, the standard connection types
// for ServerSentEvents, Websockets and common net.Conn connections can be unblocked.
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

// RWJobResult can be used to send the result of an io.Writer / io.Reader operation over a channel.
// Use it for special connection types, where ReadWriteWithContext does not fit all needs.
type RWJobResult struct {
	n   int
	err error
}
