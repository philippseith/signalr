package signalr

import (
	"bytes"
	"golang.org/x/net/websocket"
)

type WebSocketHubConnection struct {
	HubConnectionBase
}

func NewWebSocketHubConnection(protocol HubProtocol, connectionID string, conn *websocket.Conn) *WebSocketHubConnection {
	return &WebSocketHubConnection{
		HubConnectionBase: HubConnectionBase{
			ConnectionID: connectionID,
			Protocol:     protocol,
			Connected:    0,
			Writer:       conn,
			Reader:       &webSocketMessageReader{ws: conn,},
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

