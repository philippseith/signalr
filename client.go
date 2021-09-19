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

	"github.com/teivah/onecontext"

	"github.com/go-kit/log"
)

// Client is the signalR connection used on the client side.
//   Start() error
// Start starts the client loop. After starting the client, the interaction with a server can be started.
//  Stop() error
// Stop stops the client loop.
//  Context() context.Context
// Context returns a Context that is canceled when the client loop ends. Context().Err() is the error which caused
// the loop to end.
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
	Start() error
	Stop() error
	Context() context.Context
	Invoke(method string, arguments ...interface{}) <-chan InvokeResult
	Send(method string, arguments ...interface{}) <-chan error
	PullStream(method string, arguments ...interface{}) <-chan InvokeResult
	PushStreams(method string, arguments ...interface{}) <-chan error
}

// NewClient builds a new Client.
func NewClient(ctx context.Context, conn Connection, options ...func(Party) error) (Client, error) {
	info, dbg := buildInfoDebugLogger(log.NewLogfmtLogger(os.Stderr), true)
	c := &client{
		conn:      conn,
		format:    "json",
		partyBase: newPartyBase(ctx, info, dbg),
		lastID:    -1,
	}
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
	conn       Connection
	loopCtx    *clientContext
	cancelLoop context.CancelFunc
	format     string
	loop       *loop
	receiver   interface{}
	lastID     int64
	loopMx     sync.Mutex
	loopEnded  bool
}

func (c *client) Start() error {
	protocol, err := c.processHandshake()
	if err != nil {
		return err
	}

	started := make(chan struct{}, 1)

	go func(c *client, started chan struct{}) {
		c.loopMx.Lock()
		ctx, cancel := onecontext.Merge(c.ctx, c.conn.Context())
		c.loopCtx = &clientContext{Context: ctx}
		c.cancelLoop = cancel
		c.loop = newLoop(c, c.conn, protocol)
		c.loopMx.Unlock()

		c.loopCtx.SetErr(c.loop.Run(started))

		c.loopMx.Lock()
		c.loopEnded = true
		c.cancelLoop()
		c.loopMx.Unlock()
	}(c, started)

	<-started

	return nil
}

func (c *client) Stop() error {
	err := c.loop.hubConn.Close("", false)
	c.cancelLoop()
	return err
}

func (c *client) Context() context.Context {
	return c.loopCtx
}

func (c *client) Invoke(method string, arguments ...interface{}) <-chan InvokeResult {
	if ok, ch, _ := c.isLoopEnded(); ok {
		return ch
	}
	id := c.loop.GetNewID()
	resultChan, errChan := c.loop.invokeClient.newInvocation(id)
	ch := newInvokeResultChan(resultChan, errChan)
	if err := c.loop.hubConn.SendInvocation(id, method, arguments); err != nil {
		// When we get an error here, the loop is closed and the errChan might be already closed
		// We create a new one to deliver our error
		ch, _ = createResultChansWithError(err)
		c.loop.invokeClient.deleteInvocation(id)
	}
	return ch
}

func (c *client) Send(method string, arguments ...interface{}) <-chan error {
	if ok, _, ch := c.isLoopEnded(); ok {
		return ch
	}
	id := c.loop.GetNewID()
	_, errChan := c.loop.invokeClient.newInvocation(id)
	err := c.loop.hubConn.SendInvocation(id, method, arguments)
	if err != nil {
		_, errChan = createResultChansWithError(err)
		c.loop.invokeClient.deleteInvocation(id)
	}
	return errChan
}

func (c *client) PullStream(method string, arguments ...interface{}) <-chan InvokeResult {
	if ok, ch, _ := c.isLoopEnded(); ok {
		return ch
	}
	return c.loop.PullStream(method, c.loop.GetNewID(), arguments...)
}

func (c *client) PushStreams(method string, arguments ...interface{}) <-chan error {
	if ok, _, ch := c.isLoopEnded(); ok {
		return ch
	}
	return c.loop.PushStreams(method, c.loop.GetNewID(), arguments...)
}

func (c *client) isLoopEnded() (bool, <-chan InvokeResult, <-chan error) {
	defer c.loopMx.Unlock()
	c.loopMx.Lock()
	loopEnded := c.loopEnded
	if loopEnded {
		irCh, errCh := createResultChansWithError(errors.New("message loop ended"))
		return true, irCh, errCh
	}
	return false, nil, nil
}

func createResultChansWithError(err error) (<-chan InvokeResult, chan error) {
	resultChan := make(chan interface{}, 1)
	errChan := make(chan error, 1)
	errChan <- err
	invokeResultChan := newInvokeResultChan(resultChan, errChan)
	close(errChan)
	close(resultChan)
	return invokeResultChan, errChan
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
