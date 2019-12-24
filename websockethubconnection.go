package signalr

import (
	"bytes"
	"golang.org/x/net/websocket"
)

type webSocketHubConnection struct {
	baseHubConnection
}

func newWebSocketHubConnection(protocol HubProtocol, connectionID string, conn *websocket.Conn) *webSocketHubConnection {
	return &webSocketHubConnection{
		baseHubConnection: baseHubConnection{
			connectionID: connectionID,
			protocol:     protocol,
			connected:    0,
			writer:       conn,
			reader: &webSocketMessageReader{ws: conn,},
		},
	}
}

type webSocketMessageReader struct {
	ws *websocket.Conn
	r  *bytes.Reader
}

func (w *webSocketMessageReader) Read(p []byte) (n int, err error) {
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

