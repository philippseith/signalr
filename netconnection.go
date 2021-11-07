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
	n, err = ReadWriteWithContext(nc.Context(),
		func() (int, error) { return nc.conn.Write(p) },
		func() { _ = nc.conn.SetWriteDeadline(time.Now()) })
	if err != nil {
		err = fmt.Errorf("%T: %w", nc, err)
	}
	return n, err
}

func (nc *netConnection) Read(p []byte) (n int, err error) {
	n, err = ReadWriteWithContext(nc.Context(),
		func() (int, error) { return nc.conn.Read(p) },
		func() { _ = nc.conn.SetReadDeadline(time.Now()) })
	if err != nil {
		err = fmt.Errorf("%T: %w", nc, err)
	}
	return n, err
}

func getConnectionID() string {
	bytes := make([]byte, 16)
	_, _ = rand.Read(bytes)
	return base64.StdEncoding.EncodeToString(bytes)
}
