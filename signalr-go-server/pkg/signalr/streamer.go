package signalr

import "reflect"

type streamer interface {
	Start(invocationID string, reflectedChannel reflect.Value)
	Stop(invocationID string)
}

func newStreamer(conn webSocketHubConnection) streamer {
	return &defaultStreamer{make(map[string]chan bool), conn }
}

type defaultStreamer struct {
	streamCancels map[string]chan bool
	conn          webSocketHubConnection
}

func (d *defaultStreamer) Start(invocationID string, reflectedChannel reflect.Value) {
	cancelChan := make(chan bool)
	d.streamCancels[invocationID] = cancelChan
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
				d.conn.streamItem(invocationID, chanResult.Interface())
			} else {
				d.conn.completion(invocationID, nil, "")
				break
			}
		}
	}(cancelChan)
}

func (d *defaultStreamer) Stop(invocationID string) {
	if cancel, ok := d.streamCancels[invocationID]; ok {
		go func() {
			// in goroutine, because cancel might not be read when stream producer hangs
			cancel <- true
			delete(d.streamCancels, invocationID)
		}()
	}
}

