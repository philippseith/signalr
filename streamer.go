package signalr

import (
	"reflect"
	"sync"
	"sync/atomic"
)

type streamer struct {
	cancels     sync.Map
	cancelCount int64 // atomic; tracks entries in cancels for the cap check
	maxCancels  uint
	conn        HubConnection
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
	if s.maxCancels > 0 && atomic.LoadInt64(&s.cancelCount) >= int64(s.maxCancels) {
		return
	}
	if _, loaded := s.cancels.LoadOrStore(invocationID, struct{}{}); !loaded {
		atomic.AddInt64(&s.cancelCount, 1)
	}
}
