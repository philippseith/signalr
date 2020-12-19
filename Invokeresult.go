package signalr

// InvokeResult is the combined value/error result for async invocations. Used as channel type.
type InvokeResult struct {
	Value interface{}
	Error error
}

// newInvokeResultChan combines a value result and an error result channel into one InvokeResult channel
func newInvokeResultChan(resultChan <-chan interface{}, errChan <-chan error) <-chan InvokeResult {
	ch := make(chan InvokeResult, 1)
	go func(ch chan InvokeResult, resultChan <-chan interface{}, errChan <-chan error) {
		for resultChan != nil || errChan != nil {
			//goland:noinspection GoNilness
			select {
			case value, ok := <-resultChan:
				if !ok {
					resultChan = nil
				} else {
					ch <- InvokeResult{
						Value: value,
					}
				}
			case err, ok := <-errChan:
				if !ok {
					errChan = nil
				} else {
					ch <- InvokeResult{
						Error: err,
					}
				}
			}
		}
		close(ch)
	}(ch, resultChan, errChan)
	return ch
}
