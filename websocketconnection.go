package signalr

import (
	"bytes"
	"golang.org/x/net/websocket"
)

type webSocketConnection struct {
	ws *websocket.Conn
	r  *bytes.Reader
	connectionID string
}

func (w *webSocketConnection) ConnectionId() string {
	return w.connectionID
}

func (w *webSocketConnection) Write(p []byte) (n int, err error) {
	return w.ws.Write(p)
}

func (w *webSocketConnection) Read(p []byte) (n int, err error) {
	if w.r == nil || w.r.Len() == 0 {
		var data []byte
		if err = websocket.Message.Receive(w.ws, &data); err != nil {
			return 0, err
		} else {
			w.r = bytes.NewReader(data)
			return w.r.Read(p)
		}
	} else {
		return w.r.Read(p)
	}
}

