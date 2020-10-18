package signalr

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

type invokeClient struct {
	mx                 sync.Mutex
	resultChans        map[string]invokeResult
	chanReceiveTimeout time.Duration
}

func newInvokeClient(chanReceiveTimeout time.Duration) *invokeClient {
	return &invokeClient{
		mx:                 sync.Mutex{},
		resultChans:        make(map[string]invokeResult),
		chanReceiveTimeout: chanReceiveTimeout,
	}
}

type invokeResult struct {
	resultChan chan interface{}
	errChan    chan error
}

func (i *invokeClient) newInvocation(id string) (chan interface{}, chan error) {
	i.mx.Lock()
	defer i.mx.Unlock()
	r := invokeResult{
		resultChan: make(chan interface{}, 1),
		errChan:    make(chan error, 1),
	}
	i.resultChans[id] = r
	return r.resultChan, r.errChan
}

func (i *invokeClient) deleteInvocation(id string) {
	i.mx.Lock()
	defer i.mx.Unlock()
	if r, ok := i.resultChans[id]; ok {
		close(r.resultChan)
		close(r.errChan)
	}
	delete(i.resultChans, id)
}

func (i *invokeClient) handlesInvocationID(invocationID string) bool {
	_, ok := i.resultChans[invocationID]
	return ok
}

func (i *invokeClient) receiveCompletionItem(completion completionMessage) error {
	if ir, ok := i.resultChans[completion.InvocationID]; ok {
		if completion.Error != "" {
			done := make(chan struct{})
			go func() {
				ir.errChan <- errors.New(completion.Error)
				done <- struct{}{}
			}()
			select {
			case <-done:
				return nil
			case <-time.After(i.chanReceiveTimeout):
				return &hubChanTimeoutError{fmt.Sprintf("timeout (%v) waiting for hub to receive client sent error", i.chanReceiveTimeout)}
			}
		}
		if completion.Result != nil {
			done := make(chan struct{})
			go func() {
				ir.resultChan <- completion.Result
				done <- struct{}{}
			}()
			select {
			case <-done:
				return nil
			case <-time.After(i.chanReceiveTimeout):
				return &hubChanTimeoutError{fmt.Sprintf("timeout (%v) waiting for hub to receive client sent value", i.chanReceiveTimeout)}
			}
		}
		return nil
	}
	return fmt.Errorf(`unknown completion id "%v"`, completion.InvocationID)
}
