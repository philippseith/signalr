package signalr

type InvokeResult struct {
	Value interface{}
	Error error
}

func MakeInvokeResultChan(resultChan <-chan interface{}, errChan <-chan error) <-chan InvokeResult {
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
