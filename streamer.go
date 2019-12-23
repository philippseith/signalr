package signalr

import "reflect"

func newStreamer(conn hubConnection) *streamer {
	return &streamer{make(map[string]chan bool), conn }
}

type streamer struct {
	streamCancels map[string]chan bool
	conn          hubConnection
}

func (s *streamer) Start(invocationID string, reflectedChannel reflect.Value) {
	cancelChan := make(chan bool)
	s.streamCancels[invocationID] = cancelChan
	go func(cancelChan chan bool) {
		defer close(cancelChan)
		for {
			// Waits for channel, so might hang
			if chanResult, ok := reflectedChannel.Recv(); ok {
				select {
				case <-cancelChan:
					return
				default:
				}
				s.conn.streamItem(invocationID, chanResult.Interface())
			} else {
				s.conn.completion(invocationID, nil, "")
				break
			}
		}
	}(cancelChan)
}

func (s *streamer) Stop(invocationID string) {
	if cancel, ok := s.streamCancels[invocationID]; ok {
		go func() {
			// in goroutine, because cancel might not be read when stream producer hangs
			cancel <- true
			delete(s.streamCancels, invocationID)
		}()
	}
}

