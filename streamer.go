package signalr

import (
	"reflect"
	"sync"
)

func newStreamer(conn hubConnection, info StructuredLogger) *streamer {
	return &streamer{make(map[string]chan struct{}), sync.Mutex{}, conn}
}

type streamer struct {
	streamCancelChans map[string]chan struct{}
	sccMutex          sync.Mutex
	conn              hubConnection
}

func (s *streamer) Start(invocationID string, reflectedChannel reflect.Value) {
	cancelChan := make(chan struct{}, 1)
	s.sccMutex.Lock()
	s.streamCancelChans[invocationID] = cancelChan
	s.sccMutex.Unlock()
	go func(cancelChan chan struct{}) {
	loop:
		for {
			// Waits for channel, so might hang
			if chanResult, ok := reflectedChannel.Recv(); ok {
				select {
				case <-cancelChan:
					_ = s.conn.Completion(invocationID, nil, "")
					break loop
				default:
				}
				_ = s.conn.StreamItem(invocationID, chanResult.Interface())
			} else {
				_ = s.conn.Completion(invocationID, nil, "")
				break loop
			}
		}
		s.sccMutex.Lock()
		if _, ok := s.streamCancelChans[invocationID]; ok {
			delete(s.streamCancelChans, invocationID)
			close(cancelChan)
		}
		s.sccMutex.Unlock()
	}(cancelChan)
}

func (s *streamer) Stop(invocationID string) {
	// in goroutine, because cancel might not be read when stream producer hangs
	go func() {
		s.sccMutex.Lock()
		cancel, ok := s.streamCancelChans[invocationID]
		delete(s.streamCancelChans, invocationID)
		if ok {
			cancel <- struct{}{}
			close(cancel)
		}
		s.sccMutex.Unlock()
	}()
}
