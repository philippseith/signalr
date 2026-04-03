package signalr

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"github.com/coder/websocket"
)

func NewWebSocketConnection(ctx context.Context, reqURL *url.URL, connectionID string, headers http.Header) (Connection, error) {
	ws, _, err := websocket.Dial(ctx, reqURL.String(), &websocket.DialOptions{HTTPHeader: headers})
	if err != nil {
		return nil, err
	}

	return newWebSocketConnection(ctx, connectionID, ws), nil
}

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
		_ = w.conn.Close(1000, err.Error())
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
		_ = w.conn.Close(1000, err.Error())
	}
	return n, err
}

func (w *webSocketConnection) TransferMode() TransferMode {
	return w.transferMode
}

func (w *webSocketConnection) SetTransferMode(transferMode TransferMode) {
	w.transferMode = transferMode
}
