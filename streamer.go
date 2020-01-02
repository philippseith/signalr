package signalr

import (
	"reflect"
	"sync"
)

func newStreamer(conn hubConnection) *streamer {
	return &streamer{make(map[string]chan bool), sync.Mutex{}, conn}
}

type streamer struct {
	streamCancelChans map[string]chan bool
	sccMutex          sync.Mutex
	conn              hubConnection
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
				select {
				case <-cancelChan:
					s.conn.Completion(invocationID, nil, "")
					return
				default:
				}
				s.conn.StreamItem(invocationID, chanResult.Interface())
			} else {
				s.conn.Completion(invocationID, nil, "")
				break
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
