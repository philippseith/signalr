package signalr

import (
	"context"
	"errors"
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
	if client.State() == waitFor {
		close(ch)
		return ch
	}
	stateCh := make(chan struct{}, 1)
	client.PushStateChanged(stateCh)
	go func() {
		defer close(ch)
		if client.State() == waitFor {
			return
		}
		for {
			select {
			case <-stateCh:
				switch client.State() {
				case waitFor:
					return
				case ClientCreated:
					ch <- errors.New("client not started. Call client.Start() before using it")
					return
				case ClientClosed:
					ch <- errors.New("client closed and no AutoReconnect option given. Cannot reconnect")
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
