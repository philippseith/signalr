package signalr

import (
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
	"time"
)

type serverSentEventConnection struct {
	baseConnection
	mx          sync.Mutex
	postWriting bool
	postWriter  io.Writer
	postReader  io.Reader
	sseWriter   io.Writer
	sseFlusher  http.Flusher
}

func newServerSentEventConnection(connectionID string, writer http.ResponseWriter) *serverSentEventConnection {
	s := serverSentEventConnection{
		baseConnection: baseConnection{connectionID: connectionID},
		sseWriter:      writer,
		sseFlusher:     writer.(http.Flusher),
	}
	s.postReader, s.postWriter = io.Pipe()
	return &s
}

func (s *serverSentEventConnection) consumeRequest(request *http.Request) int {
	if err := s.context().Err(); err != nil {
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

func (s *serverSentEventConnection) Read(p []byte) (n int, err error) {
	if err := s.context().Err(); err != nil {
		return 0, err
	}
	return s.postReader.Read(p)
}

func (s *serverSentEventConnection) Write(p []byte) (n int, err error) {
	if err := s.context().Err(); err != nil {
		return 0, err
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
