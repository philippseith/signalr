package signalr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-kit/kit/log"
	"os"
	"reflect"
	"sync"
)

// Client is the signalR connection used on the client side
type Client interface {
	Party
	Start() error
	Stop() error
	// Closed() <-chan error TODO Define connection state
	Invoke(method string, arguments ...interface{}) <-chan InvokeResult
	Send(method string, arguments ...interface{}) <-chan error
	PullStream(method string, arguments ...interface{}) <-chan InvokeResult
	PushStreams(method string, arguments ...interface{}) <-chan error
}

// NewClient build a new Client.
// conn is a transport connection.
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
	conn      Connection
	format    string
	loop      *loop
	receiver  interface{}
	lastID    int64
	loopMx    sync.Mutex
	loopEnded bool
}

func (c *client) Start() error {
	protocol, err := c.processHandshake()
	if err != nil {
		return err
	}
	c.loop = newLoop(c, c.conn, protocol)
	started := make(chan struct{}, 1)
	go func(c *client, started chan struct{}) {
		c.loop.Run(started)
		c.loopMx.Lock()
		c.loopEnded = true
		c.loopMx.Unlock()
	}(c, started)
	<-started
	return nil
}

func (c *client) Stop() error {
	err := c.loop.hubConn.Close("", false)
	c.cancel()
	return err
}

func (c *client) Invoke(method string, arguments ...interface{}) <-chan InvokeResult {
	if ok, ch, _ := c.isLoopEnded(); ok {
		return ch
	}
	id := c.GetNewID()
	resultChan, errChan := c.loop.invokeClient.newInvocation(id)
	ch := MakeInvokeResultChan(resultChan, errChan)
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
	id := c.GetNewID()
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
	id := c.GetNewID()
	_, errChan := c.loop.invokeClient.newInvocation(id)
	upChan := c.loop.streamClient.newUpstreamChannel(id)
	ch := MakeInvokeResultChan(upChan, errChan)
	if err := c.loop.hubConn.SendStreamInvocation(id, method, arguments, nil); err != nil {
		// When we get an error here, the loop is closed and the errChan might be already closed
		// We create a new one to deliver our error
		ch, _ = createResultChansWithError(err)
		c.loop.streamClient.deleteUpstreamChannel(id)
		c.loop.invokeClient.deleteInvocation(id)
	}
	return ch
}

func (c *client) PushStreams(method string, arguments ...interface{}) <-chan error {
	if ok, _, ch := c.isLoopEnded(); ok {
		return ch
	}
	id := c.GetNewID()
	_, errChan := c.loop.invokeClient.newInvocation(id)
	invokeArgs := make([]interface{}, 0)
	reflectedChannels := make([]reflect.Value, 0)
	streamIds := make([]string, 0)
	// Parse arguments for channels and other kind of arguments
	for _, arg := range arguments {
		if reflect.TypeOf(arg).Kind() == reflect.Chan {
			reflectedChannels = append(reflectedChannels, reflect.ValueOf(arg))
			streamIds = append(streamIds, c.GetNewID())
		} else {
			invokeArgs = append(invokeArgs, arg)
		}
	}
	// Tell the server we are streaming now
	if err := c.loop.hubConn.SendStreamInvocation(c.GetNewID(), method, invokeArgs, streamIds); err != nil {
		// When we get an error here, the loop is closed and the errChan might be already closed
		// We create a new one to deliver our error
		_, errChan = createResultChansWithError(err)
		c.loop.invokeClient.deleteInvocation(id)
		return errChan
	}
	// Start streaming on all channels
	for i, reflectedChannel := range reflectedChannels {
		c.loop.streamer.Start(streamIds[i], reflectedChannel)
	}
	return errChan
}


// GetNewID returns a new, connection-unique id for invocations and streams
func (c *client) GetNewID() string {
	c.lastID++
	return fmt.Sprint(c.lastID)
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
	invokeResultChan := MakeInvokeResultChan(resultChan, errChan)
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
	rawHandshake, err := parseTextMessageFormat(c.conn, &remainBuf)
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
