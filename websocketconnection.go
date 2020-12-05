package signalr

import (
	"bytes"
	"context"
	"github.com/gorilla/websocket"
	"github.com/rotisserie/eris"
	"github.com/teivah/onecontext"
	"sync"
	"time"
)

type webSocketConnection struct {
	baseConnection
	conn    *websocket.Conn
	writeMx sync.Mutex
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
	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()
	return w
}

func (w *webSocketConnection) Write(p []byte) (n int, err error) {
	if err := w.Context().Err(); err != nil {
		return 0, eris.Wrap(err, "webSocketConnection canceled")
	}
	if w.timeout > 0 {
		defer func() { _ = w.conn.SetWriteDeadline(time.Time{}) }()
		_ = w.conn.SetWriteDeadline(time.Now().Add(w.Timeout()))
	}
	w.writeMx.Lock()
	err = w.conn.WriteMessage(websocket.TextMessage, p)
	w.writeMx.Unlock()
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func (w *webSocketConnection) Read(p []byte) (n int, err error) {
	if err := w.Context().Err(); err != nil {
		return 0, eris.Wrap(err, "webSocketConnection canceled")
	}
	if w.timeout > 0 {
		defer func() { _ = w.conn.SetReadDeadline(time.Time{}) }()
		_ = w.conn.SetReadDeadline(time.Now().Add(w.Timeout()))
	}
	_, data, err := w.conn.ReadMessage()
	if err != nil {
		return 0, err
	}
	return bytes.NewReader(data).Read(p)
}
