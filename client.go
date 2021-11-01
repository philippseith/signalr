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

	"github.com/go-kit/log"
)

// Client is the signalR connection used on the client side.
//   Start()
// Start starts the client loop. After starting the client, the interaction with a server can be started.
// The client loop will run until the server closes the connection. If WithAutoReconnect is used, Start will
// start a new loop. To end the loop from the client side, the context passed to NewClient has to be canceled.
//  State() ClientState
// State returns the current client state.
// When WithAutoReconnect is set, the client leaves ClientClosed and tries to reach ClientConnected after the last
// connection has ended.
//  PushStateChanged(chan<- struct{})
// PushStateChanged pushes a new item != nil to the channel when State has changed.
//  Err() error
// Err returns the last error occurred while running the client.
// When the client goes to ClientConnecting, Err is set to nil
//  Invoke(method string, arguments ...interface{}) <-chan InvokeResult
// Invoke invokes a method on the server and returns a channel wich will return the InvokeResult.
// When failing, InvokeResult.Error contains the client side error.
//  Send(method string, arguments ...interface{}) <-chan error
// Send invokes a method on the server but does not return a result from the server but only a channel,
// which might contain a client side error occurred while sending.
//   PullStream(method string, arguments ...interface{}) <-chan InvokeResult
// PullStream invokes a streaming method on the server and returns a channel which delivers the stream items.
// For more info about Streaming see https://github.com/dotnet/aspnetcore/blob/main/src/SignalR/docs/specs/HubProtocol.md#streaming
//   PushStreams(method string, arguments ...interface{}) <-chan error
// PushStreams pushes all items received from its arguments of type channel to the server (Upload Streaming).
// For more info about Upload Streaming see https://github.com/dotnet/aspnetcore/blob/main/src/SignalR/docs/specs/HubProtocol.md#upload-streaming
type Client interface {
	Party
	Start()
	State() ClientState
	PushStateChanged(chan<- struct{})
	Err() error
	Invoke(method string, arguments ...interface{}) <-chan InvokeResult
	Send(method string, arguments ...interface{}) <-chan error
	PullStream(method string, arguments ...interface{}) <-chan InvokeResult
	PushStreams(method string, arguments ...interface{}) <-chan error
}

// NewClient builds a new Client.
// When ctx is canceled, the client loop and a possible auto reconnect loop are ended.
// When the WithAutoReconnect option is given, conn must be nil.
func NewClient(ctx context.Context, options ...func(Party) error) (Client, error) {
	info, dbg := buildInfoDebugLogger(log.NewLogfmtLogger(os.Stderr), true)
	c := &client{
		state:            ClientCreated,
		stateChangeChans: make([]chan<- struct{}, 0),
		format:           "json",
		partyBase:        newPartyBase(ctx, info, dbg),
		lastID:           -1,
	}
	for _, option := range options {
		if option != nil {
			if err := option(c); err != nil {
				return nil, err
			}
		}
	}
	if c.conn == nil && c.connectionFactory == nil {
		return nil, errors.New("neither WithConnection nor WithAutoReconnect option was given")
	}
	return c, nil
}

type client struct {
	partyBase
	mx                sync.Mutex
	conn              Connection
	connectionFactory func() (Connection, error)
	state             ClientState
	stateChangeChans  []chan<- struct{}
	err               error
	format            string
	loop              *loop
	receiver          interface{}
	lastID            int64
}

func (c *client) Start() {
	c.setState(ClientConnecting)
	go func() {
		for {
			c.mx.Lock()
			c.err = nil
			c.mx.Unlock()
			if c.State() != ClientConnecting {
				c.setState(ClientConnecting)
			}
			loopErrChan := make(chan error, 1)
			go func() {
				for err := range loopErrChan {
					if err != nil {
						c.err = err
					}
				}
			}()
			// RUN!
			err := c.run()
			c.mx.Lock()
			c.err = err
			c.mx.Unlock()
			if c.err != nil {
				c.setState(ClientError)
			}
			// Canceled?
			if c.ctx.Err() != nil {
				c.mx.Lock()
				c.err = c.ctx.Err()
				c.mx.Unlock()
				c.setState(ClientError)
				return
			}
			// Reconnecting not possible
			if c.connectionFactory == nil {
				c.setState(ClientClosed)
				return
			}
			// Reconnecting not allowed
			if c.loop != nil && c.loop.closeMessage != nil && !c.loop.closeMessage.AllowReconnect {
				c.setState(ClientClosed)
				return
			}
		}
	}()
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
		close(isLoopConnected)
		c.setState(ClientConnected)
	}()
	// Run the loop
	err = loop.Run(isLoopConnected)
	if err != nil {
		return err
	}
	err = loop.hubConn.Close("", false) // allowReconnect value is ignored as servers never initiate a connection
	// Reset conn to allow reconnecting
	c.mx.Lock()
	c.conn = nil
	c.mx.Unlock()

	return err
}

func (c *client) setupConnectionAndProtocol() (hubProtocol, error) {
	return func() (hubProtocol, error) {
		c.mx.Lock()
		defer c.mx.Unlock()

		if c.conn == nil {
			if c.connectionFactory == nil {
				return nil, errors.New("neither Connection nor WithAutoReconnect connectionFactory set")
			}
			var err error
			c.conn, err = c.connectionFactory()
			if err != nil {
				return nil, err
			}
		}
		protocol, err := c.processHandshake()
		if err != nil {
			return nil, err
		}

		return protocol, nil
	}()
}

func (c *client) State() ClientState {
	c.mx.Lock()
	defer c.mx.Unlock()
	return c.state
}

func (c *client) PushStateChanged(ch chan<- struct{}) {
	c.mx.Lock()
	defer c.mx.Unlock()
	c.stateChangeChans = append(c.stateChangeChans, ch)
}
func (c *client) Err() error {
	c.mx.Lock()
	defer c.mx.Unlock()
	return c.err
}

func (c *client) setState(state ClientState) {
	c.mx.Lock()
	defer c.mx.Unlock()
	c.state = state
	for _, ch := range c.stateChangeChans {
		c.castStateChange(ch)
	}
}

func (c *client) castStateChange(ch chan<- struct{}) {
	go func() {
		defer func() {
			if err := recover(); err != nil {
				c.mx.Lock()
				defer c.mx.Unlock()
				for i, cch := range c.stateChangeChans {
					if cch == ch {
						c.stateChangeChans = append(c.stateChangeChans[:i], c.stateChangeChans[i+1:]...)
						break
					}
				}
			}
		}()
		select {
		case ch <- struct{}{}:
		case <-c.ctx.Done():
		}
	}()
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
		irCh := newInvokeResultChan(resultCh, errCh)
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
			}
			close(irCh)
		}()
	}()
	return irCh
}

func (c *client) PushStreams(method string, arguments ...interface{}) <-chan error {
	errCh := make(chan error, 1)
	go func() {
		if err := <-c.waitForConnected(); err != nil {
			errCh <- err
			close(errCh)
			return
		}
		pushCh, err := c.loop.PushStreams(method, c.loop.GetNewID(), arguments...)
		if err != nil {
			errCh <- err
			close(errCh)
			return
		}
		go func() {
			for err := range pushCh {
				errCh <- err
			}
			close(errCh)
		}()
	}()
	return errCh
}

func (c *client) waitForConnected() <-chan error {
	ch := make(chan error, 1)
	go func() {
		defer close(ch)
		switch c.State() {
		case ClientConnected:
			return
		case ClientCreated:
			ch <- errors.New("client not started. Call client.Start() before using it")
			return
		case ClientClosed:
			if c.connectionFactory == nil {
				ch <- errors.New("client closed and no AutoReconnect option given. Cannot reconnect")
			} else {
				c.forwardWaitForClientStateConnected(ch)
			}
		case ClientConnecting:
			c.forwardWaitForClientStateConnected(ch)
		}
	}()
	return ch
}

func (c *client) forwardWaitForClientStateConnected(ch chan error) {
	select {
	case err := <-WaitForClientState(context.Background(), c, ClientConnected):
		ch <- err
	case <-c.context().Done():
		ch <- c.context().Err()
	}
}

func createResultChansWithError(err error) (<-chan InvokeResult, chan error) {
	resultCh := make(chan interface{}, 1)
	errCh := make(chan error, 1)
	errCh <- err
	invokeResultChan := newInvokeResultChan(resultCh, errCh)
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
	info, dbg := c.prefixLoggers(c.conn.ConnectionID())
	request := fmt.Sprintf("{\"protocol\":\"%v\",\"version\":1}\u001e", c.format)
	_, err := c.conn.Write([]byte(request))
	if err != nil {
		_ = info.Log(evt, "handshake sent", "msg", request, "error", err)
		return nil, err
	}
	_ = dbg.Log(evt, "handshake sent", "msg", request)
	var remainBuf bytes.Buffer
	rawHandshake, err := readJSONFrames(c.conn, &remainBuf)
	if err != nil {
		return nil, err
	}
	response := handshakeResponse{}
	if err = json.Unmarshal(rawHandshake[0], &response); err != nil {
		// Malformed handshake
		_ = info.Log(evt, "handshake received", "msg", string(rawHandshake[0]), "error", err)
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
	return nil, err
}
