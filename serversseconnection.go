package signalr

import (
	"context"
	"errors"
	"github.com/rotisserie/eris"
	"github.com/teivah/onecontext"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
	"time"
)

type serverSSEConnection struct {
	baseConnection
	mx          sync.Mutex
	postWriting bool
	postWriter  io.Writer
	postReader  io.Reader
	sseWriter   io.Writer
	sseFlusher  http.Flusher
}

func newServerSSEConnection(parentContext context.Context, requestContext context.Context,
	connectionID string, writer http.ResponseWriter) (*serverSSEConnection, error) {
	sseFlusher, ok := writer.(http.Flusher)
	if !ok {
		return nil, errors.New("connection over Server Sent Events not supported with http.ResponseWriter: http.Flusher not implemented")
	}
	ctx, _ := onecontext.Merge(parentContext, requestContext)
	s := serverSSEConnection{
		baseConnection: baseConnection{
			ctx:          ctx,
			connectionID: connectionID,
		},
		sseWriter:  writer,
		sseFlusher: sseFlusher,
	}
	s.postReader, s.postWriter = io.Pipe()
	return &s, nil
}

func (s *serverSSEConnection) consumeRequest(request *http.Request) int {
	if err := s.Context().Err(); err != nil {
		return 410 // Gone
	}
	s.mx.Lock()
	if s.postWriting {
		s.mx.Unlock()
		return 409 // Conflict
	}
	s.postWriting = true
	s.mx.Unlock()
	defer func() {
		_ = request.Body.Close()
	}()
	body, err := ioutil.ReadAll(request.Body)
	if err != nil {
		return 400 // Bad request
	} else if _, err := s.postWriter.Write(body); err != nil {
		return 500 // Server error
	}
	s.mx.Lock()
	s.postWriting = false
	s.mx.Unlock()
	<-time.After(50 * time.Millisecond)
	return 200
}

func (s *serverSSEConnection) Read(p []byte) (n int, err error) {
	if err := s.Context().Err(); err != nil {
		return 0, eris.Wrap(err, "serverSSEConnection canceled")
	}
	return s.postReader.Read(p)
}

func (s *serverSSEConnection) Write(p []byte) (n int, err error) {
	if err := s.Context().Err(); err != nil {
		return 0, eris.Wrap(err, "serverSSEConnection canceled")
	}
	payload := ""
	for _, line := range strings.Split(strings.TrimRight(string(p), "\n"), "\n") {
		payload = payload + "data: " + line + "\n"
	}
	n, err = s.sseWriter.Write([]byte(payload + "\n"))
	if err == nil {
		s.sseFlusher.Flush()
	}
	return n, err
}
