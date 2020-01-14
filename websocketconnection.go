package signalr

import (
	"bytes"
	"golang.org/x/net/websocket"
	"time"
)

type webSocketConnection struct {
	conn         *websocket.Conn
	connectionID string
	timeout      time.Duration
}

func (w *webSocketConnection) SetTimeout(timeout time.Duration) {
	w.timeout = timeout
}

func (w *webSocketConnection) Timeout() time.Duration {
	return w.timeout
}

func (w *webSocketConnection) ConnectionID() string {
	return w.connectionID
}

func (w *webSocketConnection) Write(p []byte) (n int, err error) {
	if w.timeout > 0 {
		defer w.conn.SetWriteDeadline(time.Time{})
		w.conn.SetWriteDeadline(time.Now().Add(w.timeout))
	}
	return w.conn.Write(p)
}

func (w *webSocketConnection) Read(p []byte) (n int, err error) {
	if w.timeout > 0 {
		defer w.conn.SetReadDeadline(time.Time{})
		w.conn.SetReadDeadline(time.Now().Add(w.timeout))
	}
	var data []byte
	if err = websocket.Message.Receive(w.conn, &data); err != nil {
		return 0, err
	}
	return bytes.NewReader(data).Read(p)
}
