package signalr

import (
	"bytes"
	"golang.org/x/net/websocket"
	"time"
)

type webSocketConnection struct {
	baseConnection
	conn *websocket.Conn
}

func newWebSocketConnection(connectionID string, conn *websocket.Conn) *webSocketConnection {
	w := &webSocketConnection{
		conn:           conn,
		baseConnection: baseConnection{connectionID: connectionID},
	}
	return w
}

func (w *webSocketConnection) Write(p []byte) (n int, err error) {
	if err := w.context().Err(); err != nil {
		return 0, err
	}
	if w.timeout > 0 {
		defer func() { _ = w.conn.SetWriteDeadline(time.Time{}) }()
		_ = w.conn.SetWriteDeadline(time.Now().Add(w.timeout))
	}
	return w.conn.Write(p)
}

func (w *webSocketConnection) Read(p []byte) (n int, err error) {
	if err := w.context().Err(); err != nil {
		return 0, err
	}
	if w.timeout > 0 {
		defer func() { _ = w.conn.SetReadDeadline(time.Time{}) }()
		_ = w.conn.SetReadDeadline(time.Now().Add(w.timeout))
	}
	var data []byte
	if err = websocket.Message.Receive(w.conn, &data); err != nil {
		return 0, err
	}
	return bytes.NewReader(data).Read(p)
}
