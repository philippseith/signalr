package signalr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cenkalti/backoff/v4"

	"github.com/go-kit/log"
)

// ClientState is the state of the client.
type ClientState int

// Client states
//
//	ClientCreated
//
// The Client has been created and is not started yet.
//
//	ClientConnecting
//
// The Client has been started and is negotiating the connection.
//
//	ClientConnected
//
// The Client has successfully negotiated the connection and can send and receive messages.
//
//	ClientClosed
//
// The Client is not able to send and receive messages anymore and has to be started again to be able to.
const (
	ClientCreated ClientState = iota
	ClientConnecting
	ClientConnected
	ClientClosed
)

// Client is the signalR connection used on the client side.
//
//	Start()
//
// Start starts the client loop. After starting the client, the interaction with a server can be started.
// The client loop will run until the server closes the connection. If WithConnector is used, Start will
// start a new loop. To end the loop from the client side, the context passed to NewClient has to be canceled
// or the Stop function has to be called.
//
//	Stop()
//
// Stop stops the client loop. This is an alternative to using a cancelable context on NewClient.
//
//	State() ClientState
//
// State returns the current client state.
// When WithConnector is set and the server allows reconnection, the client switches to ClientConnecting
// and tries to reach ClientConnected after the last connection has ended.
//
//	ObserveStateChanged(chan ClientState) context.CancelFunc
//
// ObserveStateChanged pushes a new item != nil to the channel when State has changed.
// The returned CancelFunc ends the observation and closes the channel.
//
//	Err() error
//
// Err returns the last error occurred while running the client.
// When the client goes to ClientConnecting, Err is set to nil.
//
//	WaitForState(ctx context.Context, waitFor ClientState) <-chan error
//
// WaitForState returns a channel for waiting on the Client to reach a specific ClientState.
// The channel either returns an error if ctx or the client has been canceled.
// or nil if the ClientState waitFor was reached.
//
//	Invoke(method string, arguments ...interface{}) <-chan InvokeResult
//
// Invoke invokes a method on the server and returns a channel wich will return the InvokeResult.
// When failing, InvokeResult.Error contains the client side error.
//
//	Send(method string, arguments ...interface{}) <-chan error
//
// Send invokes a method on the server but does not return a result from the server but only a channel,
// which might contain a client side error occurred while sending.
//
//	PullStream(method string, arguments ...interface{}) <-chan InvokeResult
//
// PullStream invokes a streaming method on the server and returns a channel which delivers the stream items.
// For more info about Streaming see https://github.com/dotnet/aspnetcore/blob/main/src/SignalR/docs/specs/HubProtocol.md#streaming
//
//	PushStreams(method string, arguments ...interface{}) <-chan error
//
// PushStreams pushes all items received from its arguments of type channel to the server (Upload Streaming).
// PushStreams does not support server methods that return a channel.
// For more info about Upload Streaming see https://github.com/dotnet/aspnetcore/blob/main/src/SignalR/docs/specs/HubProtocol.md#upload-streaming
type Client interface {
	Party
	Start()
	Stop()
	State() ClientState
	ObserveStateChanged(chan ClientState) context.CancelFunc
	Err() error
	WaitForState(ctx context.Context, waitFor ClientState) <-chan error
	Invoke(method string, arguments ...interface{}) <-chan InvokeResult
	Send(method string, arguments ...interface{}) <-chan error
	PullStream(method string, arguments ...interface{}) <-chan InvokeResult
	PushStreams(method string, arguments ...interface{}) <-chan InvokeResult
}

var ErrUnableToConnect = errors.New("neither WithConnection nor WithConnector option was given")

// NewClient builds a new Client.
// When ctx is canceled, the client loop and a possible auto reconnect loop are ended.
func NewClient(ctx context.Context, options ...func(Party) error) (Client, error) {
	var cancelFunc context.CancelFunc
	ctx, cancelFunc = context.WithCancel(ctx)
	info, dbg := buildInfoDebugLogger(log.NewLogfmtLogger(os.Stderr), true)
	c := &client{
		state:            ClientCreated,
		stateChangeChans: make([]chan ClientState, 0),
		format:           "json",
		partyBase:        newPartyBase(ctx, info, dbg),
		lastID:           -1,
		backoffFactory:   func() backoff.BackOff { return backoff.NewExponentialBackOff() },
		cancelFunc:       cancelFunc,
	}
	for _, option := range options {
		if option != nil {
			if err := option(c); err != nil {
				return nil, err
			}
		}
	}
	// Wrap logging with timestamps
	info, dbg = c.loggers()
	c.setLoggers(
		log.WithPrefix(info, "ts", log.DefaultTimestampUTC),
		log.WithPrefix(dbg, "ts", log.DefaultTimestampUTC),
	)
	if c.conn == nil && c.connectionFactory == nil {
		return nil, ErrUnableToConnect
	}
	return c, nil
}

type client struct {
	partyBase
	mx                sync.RWMutex
	conn              Connection
	connectionFactory func() (Connection, error)
	state             ClientState
	stateChangeChans  []chan ClientState
	err               error
	format            string
	loop              *loop
	receiver          interface{}
	lastID            int64
	backoffFactory    func() backoff.BackOff
	cancelFunc        context.CancelFunc
}

func (c *client) Start() {
	c.setState(ClientConnecting)
	boff := c.backoffFactory()
	go func() {
		for {
			c.setErr(nil)
			// Listen for state change to ClientConnected and signal backoff Reset then.
			stateChangeChan := make(chan ClientState, 1)
			var connected atomic.Value
			connected.Store(false)
			cancelObserve := c.ObserveStateChanged(stateChangeChan)
			go func() {
				for range stateChangeChan {
					if c.State() == ClientConnected {
						connected.Store(true)
						return
					}
				}
			}()
			// RUN!
			err := c.run()
			if err != nil {
				_ = c.info.Log("connect", fmt.Sprintf("%v", err))
				c.setErr(err)
			}
			shouldEnd := c.shouldClientEnd()
			cancelObserve()
			if shouldEnd {
				return
			}

			// When the client has connected, BackOff should be reset
			if connected.Load().(bool) {
				boff.Reset()
			}
			// Reconnect after BackOff
			nextBackoff := boff.NextBackOff()
			// Check for exceeded backoff
			if nextBackoff == backoff.Stop {
				c.setErr(errors.New("backoff exceeded"))
				return
			}
			select {
			case <-time.After(nextBackoff):
			case <-c.ctx.Done():
				return
			}
			c.setState(ClientConnecting)
		}
	}()
}

func (c *client) Stop() {
	if c.cancelFunc != nil {
		c.cancelFunc()
	}
	c.setState(ClientClosed)
}

func (c *client) run() error {
	// negotiate and so on
	protocol, err := c.setupConnectionAndProtocol()
	if err != nil {
		return err
	}

	loop := newLoop(c, c.conn, protocol)
	c.mx.Lock()
	c.loop = loop
	c.mx.Unlock()
	// Broadcast when loop is connected
	isLoopConnected := make(chan struct{}, 1)
	go func() {
		<-isLoopConnected
		c.setState(ClientConnected)
	}()
	// Run the loop
	err = loop.Run(isLoopConnected)

	if err == nil {
		err = loop.hubConn.Close("", false) // allowReconnect value is ignored as servers never initiate a connection
	}

	// Reset conn to allow reconnecting
	c.mx.Lock()
	c.conn = nil
	c.mx.Unlock()

	return err
}

func (c *client) shouldClientEnd() bool {
	// Canceled?
	if c.ctx.Err() != nil {
		c.setErr(c.ctx.Err())
		c.setState(ClientClosed)
		return true
	}
	// Reconnecting not possible
	if c.connectionFactory == nil {
		c.setState(ClientClosed)
		return true
	}
	// Reconnecting not allowed
	if c.loop != nil && c.loop.closeMessage != nil && !c.loop.closeMessage.AllowReconnect {
		c.setState(ClientClosed)
		return true
	}
	return false
}

func (c *client) setupConnectionAndProtocol() (hubProtocol, error) {
	return func() (hubProtocol, error) {
		c.mx.Lock()
		defer c.mx.Unlock()

		if c.conn == nil {
			if c.connectionFactory == nil {
				return nil, ErrUnableToConnect
			}
			var err error
			c.conn, err = c.connectionFactory()
			if err != nil {
				return nil, err
			}
		}
		// Pass maximum receive message size to a potential websocket connection
		if wsConn, ok := c.conn.(*webSocketConnection); ok {
			wsConn.conn.SetReadLimit(int64(c.maximumReceiveMessageSize()))
		}
		protocol, err := c.processHandshake()
		if err != nil {
			return nil, err
		}

		return protocol, nil
	}()
}

func (c *client) State() ClientState {
	c.mx.RLock()
	defer c.mx.RUnlock()
	return c.state
}

func (c *client) setState(state ClientState) {
	c.mx.Lock()
	defer c.mx.Unlock()

	c.state = state
	_ = c.dbg.Log("state", state)

	for _, ch := range c.stateChangeChans {
		go func(ch chan ClientState, state ClientState) {
			c.mx.Lock()
			defer c.mx.Unlock()

			for _, cch := range c.stateChangeChans {
				if cch == ch {
					select {
					case ch <- state:
					case <-c.ctx.Done():
					}
				}
			}
		}(ch, state)
	}
}

func (c *client) ObserveStateChanged(ch chan ClientState) context.CancelFunc {
	c.mx.Lock()
	defer c.mx.Unlock()

	c.stateChangeChans = append(c.stateChangeChans, ch)

	return func() {
		c.cancelObserveStateChanged(ch)
	}
}

func (c *client) cancelObserveStateChanged(ch chan ClientState) {
	c.mx.Lock()
	defer c.mx.Unlock()
	for i, cch := range c.stateChangeChans {
		if cch == ch {
			c.stateChangeChans = append(c.stateChangeChans[:i], c.stateChangeChans[i+1:]...)
			close(ch)
			break
		}
	}
}

func (c *client) Err() error {
	c.mx.RLock()
	defer c.mx.RUnlock()
	return c.err
}

func (c *client) setErr(err error) {
	c.mx.Lock()
	defer c.mx.Unlock()
	c.err = err
}

func (c *client) WaitForState(ctx context.Context, waitFor ClientState) <-chan error {
	ch := make(chan error, 1)
	if c.waitingIsOver(waitFor, ch) {
		close(ch)
		return ch
	}
	stateCh := make(chan ClientState, 1)
	cancel := c.ObserveStateChanged(stateCh)
	go func(waitFor ClientState) {
		defer close(ch)
		defer cancel()
		if c.waitingIsOver(waitFor, ch) {
			return
		}
		for {
			select {
			case <-stateCh:
				if c.waitingIsOver(waitFor, ch) {
					return
				}
			case <-ctx.Done():
				ch <- ctx.Err()
				return
			case <-c.context().Done():
				ch <- fmt.Errorf("client canceled: %w", c.context().Err())
				return
			}
		}
	}(waitFor)
	return ch
}

func (c *client) waitingIsOver(waitFor ClientState, ch chan<- error) bool {
	switch c.State() {
	case waitFor:
		return true
	case ClientCreated:
		ch <- errors.New("client not started. Call client.Start() before using it")
		return true
	case ClientClosed:
		ch <- fmt.Errorf("client closed. %w", c.Err())
		return true
	}
	return false
}

func (c *client) Invoke(method string, arguments ...interface{}) <-chan InvokeResult {
	ch := make(chan InvokeResult, 1)
	go func() {

		if err := <-c.waitForConnected(); err != nil {
			ch <- InvokeResult{Error: err}
			close(ch)
			return
		}
		id := c.loop.GetNewID()
		resultCh, errCh := c.loop.invokeClient.newInvocation(id)
		irCh := newInvokeResultChan(c.context(), resultCh, errCh)
		if err := c.loop.hubConn.SendInvocation(id, method, arguments); err != nil {
			c.loop.invokeClient.deleteInvocation(id)
			ch <- InvokeResult{Error: err}
			close(ch)
			return
		}
		go func() {
			for ir := range irCh {
				ch <- ir
			}
			close(ch)
		}()
	}()
	return ch
}

func (c *client) Send(method string, arguments ...interface{}) <-chan error {
	errCh := make(chan error, 1)
	go func() {
		if err := <-c.waitForConnected(); err != nil {
			errCh <- err
			close(errCh)
			return
		}
		id := c.loop.GetNewID()
		_, sendErrCh := c.loop.invokeClient.newInvocation(id)
		if err := c.loop.hubConn.SendInvocation(id, method, arguments); err != nil {
			c.loop.invokeClient.deleteInvocation(id)
			errCh <- err
			close(errCh)
			return
		}
		go func() {
			for ir := range sendErrCh {
				errCh <- ir
			}
			close(errCh)
		}()
	}()
	return errCh
}

func (c *client) PullStream(method string, arguments ...interface{}) <-chan InvokeResult {
	irCh := make(chan InvokeResult, 1)
	go func() {
		if err := <-c.waitForConnected(); err != nil {
			irCh <- InvokeResult{Error: err}
			close(irCh)
			return
		}
		pullCh := c.loop.PullStream(method, c.loop.GetNewID(), arguments...)
		go func() {
			for ir := range pullCh {
				irCh <- ir
				if ir.Error != nil {
					break
				}
			}
			close(irCh)
		}()
	}()
	return irCh
}

func (c *client) PushStreams(method string, arguments ...interface{}) <-chan InvokeResult {
	irCh := make(chan InvokeResult, 1)
	go func() {
		if err := <-c.waitForConnected(); err != nil {
			irCh <- InvokeResult{Error: err}
			close(irCh)
			return
		}
		pushCh, err := c.loop.PushStreams(method, c.loop.GetNewID(), arguments...)
		if err != nil {
			irCh <- InvokeResult{Error: err}
			close(irCh)
			return
		}
		go func() {
			for ir := range pushCh {
				irCh <- ir
			}
			close(irCh)
		}()
	}()
	return irCh
}

func (c *client) waitForConnected() <-chan error {
	return c.WaitForState(context.Background(), ClientConnected)
}

func createResultChansWithError(ctx context.Context, err error) (<-chan InvokeResult, chan error) {
	resultCh := make(chan interface{}, 1)
	errCh := make(chan error, 1)
	errCh <- err
	invokeResultChan := newInvokeResultChan(ctx, resultCh, errCh)
	close(errCh)
	close(resultCh)
	return invokeResultChan, errCh
}

func (c *client) onConnected(hubConnection) {}

func (c *client) onDisconnected(hubConnection) {}

func (c *client) invocationTarget(hubConnection) interface{} {
	return c.receiver
}

func (c *client) allowReconnect() bool {
	return false // Servers don't care?
}

func (c *client) prefixLoggers(connectionID string) (info StructuredLogger, dbg StructuredLogger) {
	if c.receiver == nil {
		return log.WithPrefix(c.info, "ts", log.DefaultTimestampUTC, "class", "Client", "connection", connectionID),
			log.WithPrefix(c.dbg, "ts", log.DefaultTimestampUTC, "class", "Client", "connection", connectionID)
	}
	var t reflect.Type = nil
	switch reflect.ValueOf(c.receiver).Kind() {
	case reflect.Ptr:
		t = reflect.ValueOf(c.receiver).Elem().Type()
	case reflect.Struct:
		t = reflect.ValueOf(c.receiver).Type()
	}
	return log.WithPrefix(c.info, "ts", log.DefaultTimestampUTC,
			"class", "Client",
			"connection", connectionID,
			"hub", t),
		log.WithPrefix(c.dbg, "ts", log.DefaultTimestampUTC,
			"class", "Client",
			"connection", connectionID,
			"hub", t)
}

func (c *client) processHandshake() (hubProtocol, error) {
	if err := c.sendHandshakeRequest(); err != nil {
		return nil, err
	}
	return c.receiveHandshakeResponse()
}

func (c *client) sendHandshakeRequest() error {
	info, dbg := c.prefixLoggers(c.conn.ConnectionID())
	request := fmt.Sprintf("{\"protocol\":\"%v\",\"version\":1}\u001e", c.format)
	ctx, cancelWrite := context.WithTimeout(c.context(), c.HandshakeTimeout())
	defer cancelWrite()
	_, err := ReadWriteWithContext(ctx,
		func() (int, error) {
			return c.conn.Write([]byte(request))
		}, func() {})
	if err != nil {
		_ = info.Log(evt, "handshake sent", "msg", request, "error", err)
		return err
	}
	_ = dbg.Log(evt, "handshake sent", "msg", request)
	return nil
}

func (c *client) receiveHandshakeResponse() (hubProtocol, error) {
	info, dbg := c.prefixLoggers(c.conn.ConnectionID())
	ctx, cancelRead := context.WithTimeout(c.context(), c.HandshakeTimeout())
	defer cancelRead()
	readJSONFramesChan := make(chan []interface{}, 1)
	go func() {
		var remainBuf bytes.Buffer
		rawHandshake, err := readJSONFrames(c.conn, &remainBuf)
		readJSONFramesChan <- []interface{}{rawHandshake, err}
	}()
	select {
	case result := <-readJSONFramesChan:
		if result[1] != nil {
			return nil, result[1].(error)
		}
		rawHandshake := result[0].([][]byte)
		response := handshakeResponse{}
		if err := json.Unmarshal(rawHandshake[0], &response); err != nil {
			// Malformed handshake
			_ = info.Log(evt, "handshake received", "msg", string(rawHandshake[0]), "error", err)
			return nil, err
		} else {
			if response.Error != "" {
				_ = info.Log(evt, "handshake received", "error", response.Error)
				return nil, errors.New(response.Error)
			}
			_ = dbg.Log(evt, "handshake received", "msg", fmtMsg(response))
			var protocol hubProtocol
			switch c.format {
			case "json":
				protocol = &jsonHubProtocol{}
			case "messagepack":
				protocol = &messagePackHubProtocol{}
			}
			if protocol != nil {
				_, pDbg := c.loggers()
				protocol.setDebugLogger(pDbg)
			}
			return protocol, nil
		}
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
