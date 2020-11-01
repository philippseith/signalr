package signalr

import (
	"bytes"
	"context"
	"github.com/rotisserie/eris"
	"github.com/teivah/onecontext"
	"golang.org/x/net/websocket"
	"time"
)

type webSocketConnection struct {
	baseConnection
	conn *websocket.Conn
}

func newWebSocketConnection(parentContext context.Context, requestContext context.Context, connectionID string, conn *websocket.Conn) *webSocketConnection {
	ctx, _ := onecontext.Merge(parentContext, requestContext)
	w := &webSocketConnection{
		conn: conn,
		baseConnection: baseConnection{
			ctx:          ctx,
			connectionID: connectionID,
		},
	}
	return w
}

func (w *webSocketConnection) Write(p []byte) (n int, err error) {
	if err := w.Context().Err(); err != nil {
		return 0, eris.Wrap(err, "webSocketConnection canceled")
	}
	if w.timeout > 0 {
		defer func() { _ = w.conn.SetWriteDeadline(time.Time{}) }()
		_ = w.conn.SetWriteDeadline(time.Now().Add(w.timeout))
	}
	return w.conn.Write(p)
}

func (w *webSocketConnection) Read(p []byte) (n int, err error) {
	if err := w.Context().Err(); err != nil {
		return 0, eris.Wrap(err, "webSocketConnection canceled")
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
