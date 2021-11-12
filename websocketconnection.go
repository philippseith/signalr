package signalr

import (
	"bytes"
	"context"
	"fmt"

	"nhooyr.io/websocket"
)

type webSocketConnection struct {
	ConnectionBase
	conn         *websocket.Conn
	transferMode TransferMode
}

func newWebSocketConnection(ctx context.Context, connectionID string, conn *websocket.Conn) *webSocketConnection {
	w := &webSocketConnection{
		conn:           conn,
		ConnectionBase: *NewConnectionBase(ctx, connectionID),
	}
	return w
}

func (w *webSocketConnection) Write(p []byte) (n int, err error) {
	messageType := websocket.MessageText
	if w.transferMode == BinaryTransferMode {
		messageType = websocket.MessageBinary
	}
	n, err = ReadWriteWithContext(w.Context(),
		func() (int, error) {
			err := w.conn.Write(w.Context(), messageType, p)
			if err != nil {
				return 0, err
			}
			return len(p), nil
		},
		func() {})
	if err != nil {
		err = fmt.Errorf("%T: %w", w, err)
	}
	return n, err
}

func (w *webSocketConnection) Read(p []byte) (n int, err error) {
	n, err = ReadWriteWithContext(w.Context(),
		func() (int, error) {
			_, data, err := w.conn.Read(w.Context())
			if err != nil {
				return 0, err
			}
			return bytes.NewReader(data).Read(p)
		},
		func() {})
	if err != nil {
		err = fmt.Errorf("%T: %w", w, err)
	}
	return n, err
}

func (w *webSocketConnection) TransferMode() TransferMode {
	return w.transferMode
}

func (w *webSocketConnection) SetTransferMode(transferMode TransferMode) {
	w.transferMode = transferMode
}
