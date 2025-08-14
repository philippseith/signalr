package signalr

import (
	"context"
	"fmt"
	"sync"

	"github.com/quic-go/webtransport-go"
)

type webTransportsConnection struct {
	ConnectionBase
	session *webtransport.Session

	// current stream guarded by mutex
	stream *webtransport.Stream
	sync.Mutex
}

func newWebTransportsConnection(ctx context.Context, connectionID string, session *webtransport.Session) *webTransportsConnection {
	w := &webTransportsConnection{
		session:        session,
		ConnectionBase: *NewConnectionBase(ctx, connectionID),
	}
	return w
}

func (w *webTransportsConnection) Write(p []byte) (n int, err error) {
	err = w.syncStream()
	if err != nil {
		return 0, err
	}
	n, err = ReadWriteWithContext(w.Context(),
		func() (int, error) {
			return w.stream.Write(p)
		},
		func() {})
	if err != nil {
		err = fmt.Errorf("%T: %w", w, err)
		_ = w.closeStream()
	}
	return n, err
}

func (w *webTransportsConnection) Read(p []byte) (n int, err error) {
	err = w.syncStream()
	if err != nil {
		return 0, err
	}
	n, err = ReadWriteWithContext(w.Context(),
		func() (int, error) {
			return w.stream.Read(p)
		},
		func() {})
	if err != nil {
		err = fmt.Errorf("%T: %w", w, err)
		_ = w.closeStream()
	}
	return n, err
}

func (w *webTransportsConnection) syncStream() (err error) {
	w.Lock()
	defer w.Unlock()
	if w.stream == nil {
		w.stream, err = w.session.OpenStream()
	}
	return
}

func (w *webTransportsConnection) closeStream() (err error) {
	w.Lock()
	defer w.Unlock()
	if w.stream == nil {
		return nil
	}
	return w.stream.Close()
}
