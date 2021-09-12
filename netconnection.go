package signalr

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"net"
	"time"
)

type netConnection struct {
	ConnectionBase
	conn net.Conn
}

func NewNetConnection(ctx context.Context, conn net.Conn) *netConnection {
	netConn := &netConnection{
		ConnectionBase: ConnectionBase{
			ctx:          ctx,
			connectionID: getConnectionID(),
		},
		conn: conn,
	}
	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()
	return netConn
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
