package signalr

import (
	"github.com/go-kit/kit/log"
	"reflect"
	"sync"
)

func newStreamer(conn hubConnection, info StructuredLogger) *streamer {
	info = log.WithPrefix(info, "ts", log.DefaultTimestampUTC,
		"class", "streamer",
		"connection", conn.ConnectionID())
	return &streamer{make(map[string]chan bool), sync.Mutex{}, conn, info}
}

type streamer struct {
	streamCancelChans map[string]chan bool
	sccMutex          sync.Mutex
	conn              hubConnection
	info              StructuredLogger
}

func (s *streamer) Start(invocationID string, reflectedChannel reflect.Value) {
	cancelChan := make(chan bool)
	s.sccMutex.Lock()
	defer s.sccMutex.Unlock()
	s.streamCancelChans[invocationID] = cancelChan
	go func(cancelChan chan bool) {
		defer func() {
			s.sccMutex.Lock()
			defer s.sccMutex.Unlock()
			delete(s.streamCancelChans, invocationID)
			close(cancelChan)
		}()
		for {
			// Waits for channel, so might hang
			if chanResult, ok := reflectedChannel.Recv(); ok {
				if s.conn.IsConnected() {
					select {
					case <-cancelChan:
						sendMessageAndLog(func() (i interface{}, err error) {
							return s.conn.Completion(invocationID, nil, "")
						}, s.info)
						return
					default:
					}
					sendMessageAndLog(func() (i interface{}, err error) {
						return s.conn.StreamItem(invocationID, chanResult.Interface())
					}, s.info)
				} else {
					return
				}
			} else {
				if s.conn.IsConnected() {
					sendMessageAndLog(func() (i interface{}, err error) {
						return s.conn.Completion(invocationID, nil, "")
					}, s.info)
				}
				return
			}
		}
	}(cancelChan)
}

func (s *streamer) Stop(invocationID string) {
	// in goroutine, because cancel might not be read when stream producer hangs
	go func() {
		s.sccMutex.Lock()
		defer s.sccMutex.Unlock()
		if cancel, ok := s.streamCancelChans[invocationID]; ok {
			cancel <- true
		}
	}()
}
