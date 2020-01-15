package main

import (
	"crypto/rand"
	"encoding/base64"
	"net"
	"time"
)

type netConnection struct {
	timeout      time.Duration
	conn         net.Conn
	connectionID string
}

func newNetConnection(conn net.Conn) *netConnection {
	return &netConnection{connectionID: getConnectionID(), conn: conn}
}

func (w *netConnection) SetTimeout(timeout time.Duration) {
	w.timeout = timeout
}

func (w *netConnection) Timeout() time.Duration {
	return w.timeout
}

func (w *netConnection) ConnectionID() string {
	return w.connectionID
}

func (w *netConnection) Write(p []byte) (n int, err error) {
	if w.timeout > 0 {
		defer w.conn.SetWriteDeadline(time.Time{})
		w.conn.SetWriteDeadline(time.Now().Add(w.timeout))
	}
	return w.conn.Write(p)
}

func (w *netConnection) Read(p []byte) (n int, err error) {
	if w.timeout > 0 {
		defer w.conn.SetReadDeadline(time.Time{})
		w.conn.SetReadDeadline(time.Now().Add(w.timeout))
	}
	return w.conn.Read(p)
}

func getConnectionID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return base64.StdEncoding.EncodeToString(bytes)
}
