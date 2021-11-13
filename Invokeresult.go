package signalr

import (
	"context"
)

// InvokeResult is the combined value/error result for async invocations. Used as channel type.
type InvokeResult struct {
	Value interface{}
	Error error
}

// newInvokeResultChan combines a value result and an error result channel into one InvokeResult channel
// The InvokeResult channel is automatically closed when both input channels are closed.
func newInvokeResultChan(ctx context.Context, resultChan <-chan interface{}, errChan <-chan error) <-chan InvokeResult {
	ch := make(chan InvokeResult, 1)
	go func(ctx context.Context, ch chan InvokeResult, resultChan <-chan interface{}, errChan <-chan error) {
		var resultChanClosed, errChanClosed bool
	loop:
		for !resultChanClosed || !errChanClosed {
			select {
			case <-ctx.Done():
				break loop
			case value, ok := <-resultChan:
				if !ok {
					resultChanClosed = true
				} else {
					ch <- InvokeResult{
						Value: value,
					}
				}
			case err, ok := <-errChan:
				if !ok {
					errChanClosed = true
				} else {
					ch <- InvokeResult{
						Error: err,
					}
				}
			}
		}
		close(ch)
	}(ctx, ch, resultChan, errChan)
	return ch
}
