package signalr

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"net"
	"time"
)

type netConnection struct {
	ctx          context.Context
	timeout      time.Duration
	conn         net.Conn
	connectionID string
}

func NewNetConnection(ctx context.Context, conn net.Conn) *netConnection {
	netConn := &netConnection{
		ctx:          ctx,
		connectionID: getConnectionID(),
		conn:         conn,
	}
	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()
	return netConn
}

func (nc *netConnection) SetTimeout(timeout time.Duration) {
	nc.timeout = timeout
}

func (nc *netConnection) Timeout() time.Duration {
	return nc.timeout
}

func (nc *netConnection) ConnectionID() string {
	return nc.connectionID
}

func (nc *netConnection) SetConnectionID(id string) {
	nc.connectionID = id
}

func (nc *netConnection) Context() context.Context {
	return nc.ctx
}

func (nc *netConnection) Write(p []byte) (n int, err error) {
	if nc.timeout > 0 {
		defer func() { _ = nc.conn.SetWriteDeadline(time.Time{}) }()
		_ = nc.conn.SetWriteDeadline(time.Now().Add(nc.timeout))
	}
	return nc.conn.Write(p)
}

func (nc *netConnection) Read(p []byte) (n int, err error) {
	if nc.timeout > 0 {
		defer func() { _ = nc.conn.SetReadDeadline(time.Time{}) }()
		_ = nc.conn.SetReadDeadline(time.Now().Add(nc.timeout))
	}
	return nc.conn.Read(p)
}

func getConnectionID() string {
	bytes := make([]byte, 16)
	_, _ = rand.Read(bytes)
	return base64.StdEncoding.EncodeToString(bytes)
}
