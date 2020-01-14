package signalr

import (
	"bytes"
	"golang.org/x/net/websocket"
)

type webSocketConnection struct {
	ws           *websocket.Conn
	connectionID string
}

func (w *webSocketConnection) ConnectionID() string {
	return w.connectionID
}

func (w *webSocketConnection) Write(p []byte) (n int, err error) {
	return w.ws.Write(p)
}

func (w *webSocketConnection) Read(p []byte) (n int, err error) {
	var data []byte
	if err = websocket.Message.Receive(w.ws, &data); err != nil {
		return 0, err
	}
	return bytes.NewReader(data).Read(p)
}
