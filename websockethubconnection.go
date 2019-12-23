package signalr

import (
	"bytes"
	"golang.org/x/net/websocket"
)

type webSocketHubConnection struct {
	baseHubConnection
	ws *websocket.Conn
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
		ws: conn,
	}
}

func (w *webSocketHubConnection) receive() (interface{}, error) {
	var buf bytes.Buffer
	var data []byte
	for {
		if message, err := w.protocol.ReadMessage(&buf); err != nil {
			// Partial message, need more data
			if err = websocket.Message.Receive(w.ws, &data); err != nil {
				return nil, err
			} else {
				buf.Write(data)
			}
		} else {
			return message, nil
		}
	}
}




