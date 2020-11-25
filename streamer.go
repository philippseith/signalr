package signalr

import (
	"reflect"
	"sync"
)

type streamer struct {
	cancels sync.Map
	conn    hubConnection
}

func (s *streamer) Start(invocationID string, reflectedChannel reflect.Value) {
	go func() {
	loop:
		for {
			// Waits for channel, so might hang
			if chanResult, ok := reflectedChannel.Recv(); ok {
				if _, ok := s.cancels.Load(invocationID); ok {
					s.cancels.Delete(invocationID)
					_ = s.conn.Completion(invocationID, nil, "")
					break loop
				}
				if s.conn.Context().Err() != nil {
					break loop
				}
				_ = s.conn.StreamItem(invocationID, chanResult.Interface())
			} else {
				if s.conn.Context().Err() == nil {
					_ = s.conn.Completion(invocationID, nil, "")
				}
				break loop
			}
		}
	}()
}

func (s *streamer) Stop(invocationID string) {
	s.cancels.Store(invocationID, struct{}{})
}
