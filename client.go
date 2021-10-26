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
//   Start() <- chan error
// Start starts the client loop. After starting the client, the interaction with a server can be started.
// The client loop will run until the server closes the connection. If AutoReconnect is used, Start will
// start a new loop. To end the loop from the client side, the context passed in NewClient has to be canceled.
// In that case
//  errors.Is(<-Start(), context.Canceled)
// is true.
//  WaitConnected(ctx context.Context) error
// WaitConnected waits until the Client is connected or closed or ctx was canceled.
// When closed or ctx was canceled, the result is != nil.
//  WaitClosed(ctx context.Context) error
// WaitClosed waits until the Client is closed or ctx is canceled. The result is != nil when ctx was canceled.
// The client gets in closed state when the client loop ends or can not be started.
// When AutoReconnect is set, the client leaves closed state and tries to reach connected state.
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
	Start() <-chan error
	WaitConnected(ctx context.Context) error
	WaitClosed(ctx context.Context) error
	Invoke(method string, arguments ...interface{}) <-chan InvokeResult
	Send(method string, arguments ...interface{}) <-chan error
	PullStream(method string, arguments ...interface{}) <-chan InvokeResult
	PushStreams(method string, arguments ...interface{}) <-chan error
}

// NewClient builds a new Client.
// When ctx is canceled, the client loop and a possible auto reconnect loop are ended.
// When the AutoReconnect option is given, conn must be nil.
func NewClient(ctx context.Context, conn Connection, options ...func(Party) error) (Client, error) {
	info, dbg := buildInfoDebugLogger(log.NewLogfmtLogger(os.Stderr), true)
	c := &client{
		conn:      conn,
		format:    "json",
		partyBase: newPartyBase(ctx, info, dbg),
		lastID:    -1,
	}
	c.stateCond = sync.NewCond(&c.stateMx)
	for _, option := range options {
		if option != nil {
			if err := option(c); err != nil {
				return nil, err
			}
		}
	}
	return c, nil
}

type client struct {
	partyBase
	mx                sync.Mutex
	stateMx           sync.Mutex
	stateCond         *sync.Cond
	conn              Connection
	connectionFactory func() (Connection, error)
	connected         bool
	closed            bool
	format            string
	loop              *loop
	receiver          interface{}
	lastID            int64
}

func (c *client) Start() <-chan error {
	errChan := make(chan error, 1)
	go func() {
		for {
			// reset state
			c.stateMx.Lock()
			c.connected = false
			c.closed = false
			c.stateMx.Unlock()
			// Redirect inner errors to the outside
			loopErrChan := make(chan error, 1)
			go func() {
				for err := range loopErrChan {
					errChan <- err
				}
			}()
			// RUN!
			c.run(loopErrChan)
			close(loopErrChan)
			// Canceled?
			if c.ctx.Err() != nil {
				return
			}
			// Reconnecting not possible
			if c.connectionFactory == nil {
				return
			}
			// Reconnecting not allowed
			if c.loop != nil && c.loop.closeMessage != nil && !c.loop.closeMessage.AllowReconnect {
				return
			}
		}
	}()
	return errChan
}

func (c *client) run(errChan chan<- error) {
	// When run is over, the client is closed (but might be reopened)
	defer func() {
		c.stateMx.Lock()
		c.closed = true
		c.stateCond.Broadcast()
		c.stateMx.Unlock()
	}()

	// Broadcast when loop is connected
	isLoopConnected := make(chan bool, 1)
	go func() {
		connected := <-isLoopConnected
		close(isLoopConnected)
		c.stateMx.Lock()
		c.connected = connected
		c.stateCond.Broadcast()
		c.stateMx.Unlock()
	}()

	protocol, err := c.setupConnectionAndProtocol(errChan)
	if err != nil {
		return
	}

	loop := newLoop(c, c.conn, protocol)
	c.mx.Lock()
	c.loop = loop
	c.mx.Unlock()

	err = loop.Run(isLoopConnected)
	if err != nil {
		errChan <- err
		return
	}
	err = loop.hubConn.Close("", false) // allowReconnect value is ignored as servers never initiate a connection
	// Reset conn to allow reconnecting
	c.mx.Lock()
	c.conn = nil
	c.mx.Unlock()

	if err != nil {
		errChan <- err
	}
}

func (c *client) setupConnectionAndProtocol(errChan chan<- error) (hubProtocol, error) {
	return func() (hubProtocol, error) {
		c.mx.Lock()
		defer c.mx.Unlock()

		if c.conn == nil {
			if c.connectionFactory == nil {
				err := errors.New("neither Connection nor AutoReconnect connectionFactory set")
				go func() {
					errChan <- err
				}()
				return nil, err
			}
			var err error
			c.conn, err = c.connectionFactory()
			if err != nil {
				errChan <- err
				return nil, err
			}
		}
		protocol, err := c.processHandshake()
		if err != nil {
			errChan <- err
			return nil, err
		}

		return protocol, nil
	}()
}

func (c *client) WaitConnected(ctx context.Context) error {
	c.stateMx.Lock()
	defer c.stateMx.Unlock()

	for !c.connected {
		c.stateCond.Wait()
		if c.closed {
			return errors.New("client is not establishing a connection")
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return nil
}

func (c *client) WaitClosed(ctx context.Context) error {
	c.stateMx.Lock()
	defer c.stateMx.Unlock()
	for !c.closed {
		c.stateCond.Wait()
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return nil
}

func (c *client) Invoke(method string, arguments ...interface{}) <-chan InvokeResult {
	ch := make(chan InvokeResult, 1)
	go func() {
		if err := c.WaitConnected(context.Background()); err != nil {
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
		if err := c.WaitConnected(context.Background()); err != nil {
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
		if err := c.WaitConnected(context.Background()); err != nil {
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
		if err := c.WaitConnected(context.Background()); err != nil {
			errCh <- err
			close(errCh)
			return
		}
		pushCh := c.loop.PushStreams(method, c.loop.GetNewID(), arguments...)
		go func() {
			for err := range pushCh {
				errCh <- err
			}
			close(errCh)
		}()
	}()
	return errCh
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
