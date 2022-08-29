package signalr

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

type serverSSEConnection struct {
	ConnectionBase
	mx            sync.Mutex
	postWriting   bool
	postWriter    io.Writer
	postReader    io.Reader
	jobChan       chan []byte
	jobResultChan chan RWJobResult
}

func newServerSSEConnection(ctx context.Context, connectionID string) (*serverSSEConnection, <-chan []byte, chan RWJobResult, error) {
	s := serverSSEConnection{
		ConnectionBase: *NewConnectionBase(ctx, connectionID),
		jobChan:        make(chan []byte, 1),
		jobResultChan:  make(chan RWJobResult, 1),
	}
	s.postReader, s.postWriter = io.Pipe()
	go func() {
		<-s.Context().Done()
		s.mx.Lock()
		close(s.jobChan)
		s.mx.Unlock()
	}()
	return &s, s.jobChan, s.jobResultChan, nil
}

func (s *serverSSEConnection) consumeRequest(request *http.Request) int {
	if err := s.Context().Err(); err != nil {
		return http.StatusGone // 410
	}
	s.mx.Lock()
	if s.postWriting {
		s.mx.Unlock()
		return http.StatusConflict // 409
	}
	s.postWriting = true
	s.mx.Unlock()
	defer func() {
		_ = request.Body.Close()
	}()
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return http.StatusBadRequest // 400
	} else if _, err := s.postWriter.Write(body); err != nil {
		return http.StatusInternalServerError // 500
	}
	s.mx.Lock()
	s.postWriting = false
	s.mx.Unlock()
	<-time.After(50 * time.Millisecond)
	return http.StatusOK // 200
}

func (s *serverSSEConnection) Read(p []byte) (n int, err error) {
	n, err = ReadWriteWithContext(s.Context(),
		func() (int, error) { return s.postReader.Read(p) },
		func() { _, _ = s.postWriter.Write([]byte("\n")) })
	if err != nil {
		err = fmt.Errorf("%T: %w", s, err)
	}
	return n, err
}

func (s *serverSSEConnection) Write(p []byte) (n int, err error) {
	if err := s.Context().Err(); err != nil {
		return 0, fmt.Errorf("%T: %w", s, s.Context().Err())
	}
	payload := ""
	for _, line := range strings.Split(strings.TrimRight(string(p), "\n"), "\n") {
		payload = payload + "data: " + line + "\n"
	}
	// prevent race with goroutine closing the jobChan
	s.mx.Lock()
	if s.Context().Err() == nil {
		s.jobChan <- []byte(payload + "\n")
	} else {
		return 0, fmt.Errorf("%T: %w", s, s.Context().Err())
	}
	s.mx.Unlock()
	select {
	case <-s.Context().Done():
		return 0, fmt.Errorf("%T: %w", s, s.Context().Err())
	case r := <-s.jobResultChan:
		return r.n, r.err
	}

}
