package signalr

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net"
	"time"
)

type netConnection struct {
	ConnectionBase
	conn net.Conn
}

// NewNetConnection wraps net.Conn into a Connection
func NewNetConnection(ctx context.Context, conn net.Conn) Connection {
	netConn := &netConnection{
		ConnectionBase: *NewConnectionBase(ctx, getConnectionID()),
		conn:           conn,
	}
	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()
	return netConn
}

func (nc *netConnection) Write(p []byte) (n int, err error) {
	resultChan := make(chan rwJobResult, 1)
	go func() {
		n, err := nc.conn.Write(p)
		resultChan <- rwJobResult{n: n, err: err}
		close(resultChan)
	}()
	select {
	case <-nc.ContextWithTimeout().Done():
		// Break potentially blocking Write
		_ = nc.conn.SetWriteDeadline(time.Now())
		if nc.Context().Err() != nil {
			return 0, fmt.Errorf("connection canceled %w", nc.Context().Err())
		}
		return 0, fmt.Errorf("connection Write timeout %v", nc.Timeout())
	case r := <-resultChan:
		return r.n, r.err
	}
}

func (nc *netConnection) Read(p []byte) (n int, err error) {
	resultChan := make(chan rwJobResult, 1)
	go func() {
		n, err := nc.conn.Read(p)
		resultChan <- rwJobResult{n: n, err: err}
		close(resultChan)
	}()
	select {
	case <-nc.ContextWithTimeout().Done():
		// Break potentially blocking Read
		_ = nc.conn.SetReadDeadline(time.Now())
		if nc.Context().Err() != nil {
			return 0, fmt.Errorf("connection canceled %w", nc.Context().Err())
		}
		return 0, fmt.Errorf("connection Read timeout %v", nc.Timeout())
	case r := <-resultChan:
		return r.n, r.err
	}
}

func getConnectionID() string {
	bytes := make([]byte, 16)
	_, _ = rand.Read(bytes)
	return base64.StdEncoding.EncodeToString(bytes)
}
