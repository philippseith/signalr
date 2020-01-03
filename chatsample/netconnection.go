package main

import (
	"crypto/rand"
	"encoding/base64"
	"net"
)

type netConnection struct {
	conn         net.Conn
	connectionID string
}

func newNetConnection(conn net.Conn) *netConnection {
	return &netConnection{connectionID: getConnectionID(), conn: conn}
}

func (w *netConnection) ConnectionID() string {
	return w.connectionID
}

func (w *netConnection) Write(p []byte) (n int, err error) {
	return w.conn.Write(p)
}

func (w *netConnection) Read(p []byte) (n int, err error) {
	return w.conn.Read(p)
}

func getConnectionID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return base64.StdEncoding.EncodeToString(bytes)
}
