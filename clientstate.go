package signalr

import (
	"context"
	"fmt"
)

type ClientState int

const (
	ClientCreated ClientState = iota
	ClientConnecting
	ClientConnected
	ClientClosed
	ClientError
)

// WaitForClientState returns a channel for waiting on the Client to reach a specific ClientState.
// The channel either returns an error if ctx or the client has benn canceled
// or nil if the ClientState waitFor was reached.
func WaitForClientState(ctx context.Context, client Client, waitFor ClientState) <-chan error {
	ch := make(chan error, 1)
	stateCh := make(chan struct{})
	client.PushStateChanged(stateCh)
	go func() {
		defer close(ch)
		for {
			select {
			case <-stateCh:
				if client.State() == waitFor {
					return
				}
			case <-ctx.Done():
				ch <- ctx.Err()
				return
			case <-client.context().Done():
				ch <- fmt.Errorf("client canceled: %w", client.context().Err())
				return
			}
		}
	}()
	return ch
}
