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

// ClientConnection is the signalR connection used on the client side
type ClientConnection interface {
	party
	Start() <-chan error
	Close() error
	// Closed() <-chan error TODO Define connection state
	Invoke(method string, arguments ...interface{}) <-chan InvokeResult
	Send(method string, arguments ...interface{}) <-chan error
	PullStream(method string, arguments ...interface{}) <-chan InvokeResult
	PushStreams(method string, arguments ...interface{}) <-chan error
	// It is not necessary to register callbacks with On(...),
	// the server can "call back" all exported methods of the receiver
	SetReceiver(receiver interface{})
}

// NewClientConnection build a new ClientConnection.
// conn is a transport connection.
func NewClientConnection(conn Connection, options ...func(party) error) (ClientConnection, error) {
	info, dbg := buildInfoDebugLogger(log.NewLogfmtLogger(os.Stderr), true)
	c := &clientConnection{
		conn:      conn,
		partyBase: newPartyBase(info, dbg),
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

type clientConnection struct {
	partyBase
	conn      Connection
	cancel    context.CancelFunc
	loop      *loop
	receiver  interface{}
	lastID    int64
	loopMx    sync.Mutex
	loopEnded bool
}

func (c *clientConnection) Start() <-chan error {
	errCh := make(chan error, 1)
	go func(c *clientConnection) {
		if protocol, err := c.processHandshake(); err != nil {
			errCh <- err
		} else {
			errCh <- nil
			var ctx context.Context
			ctx, c.cancel = context.WithCancel(context.Background())
			c.loop = newLoop(ctx, c, c.conn, protocol)
			c.loop.Run()
			c.loopMx.Lock()
			c.loopEnded = true
			c.loopMx.Unlock()
		}
	}(c)
	return errCh
}

func (c *clientConnection) Close() error {
	_, err := c.loop.hubConn.Close("", false)
	c.cancel()
	return err
}

func (c *clientConnection) Invoke(method string, arguments ...interface{}) <-chan InvokeResult {
	if ok, ch, _ := c.isLoopEnded(); ok {
		return ch
	}
	id := c.GetNewID()
	resultChan, errChan := c.loop.invokeClient.newInvocation(id)
	ch := MakeInvokeResultChan(resultChan, errChan)
	if _, err := c.loop.hubConn.SendInvocation(id, method, arguments); err != nil {
		// When we get an error here, the loop is closed and the errChan might be already closed
		// We create a new one to deliver our error
		ch, _ = createResultChansWithError(err)
		c.loop.invokeClient.deleteInvocation(id)
	}
	return ch
}

func (c *clientConnection) Send(method string, arguments ...interface{}) <-chan error {
	if ok, _, ch := c.isLoopEnded(); ok {
		return ch
	}
	id := c.GetNewID()
	_, errChan := c.loop.invokeClient.newInvocation(id)
	_, err := c.loop.hubConn.SendInvocation(id, method, arguments)
	if err != nil {
		_, errChan = createResultChansWithError(err)
		c.loop.invokeClient.deleteInvocation(id)
	}
	return errChan
}

func (c *clientConnection) PullStream(method string, arguments ...interface{}) <-chan InvokeResult {
	if ok, ch, _ := c.isLoopEnded(); ok {
		return ch
	}
	id := c.GetNewID()
	_, errChan := c.loop.invokeClient.newInvocation(id)
	upChan := c.loop.streamClient.newUpstreamChannel(id)
	ch := MakeInvokeResultChan(upChan, errChan)
	if _, err := c.loop.hubConn.SendStreamInvocation(id, method, arguments, nil); err != nil {
		// When we get an error here, the loop is closed and the errChan might be already closed
		// We create a new one to deliver our error
		ch, _ = createResultChansWithError(err)
		c.loop.streamClient.deleteUpstreamChannel(id)
		c.loop.invokeClient.deleteInvocation(id)
	}
	return ch
}

func (c *clientConnection) PushStreams(method string, arguments ...interface{}) <-chan error {
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
	if _, err := c.loop.hubConn.SendStreamInvocation(c.GetNewID(), method, invokeArgs, streamIds); err != nil {
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

func (c *clientConnection) SetReceiver(receiver interface{}) {
	c.receiver = receiver
}

// GetNewID returns a new, connection-unique id for invocations and streams
func (c *clientConnection) GetNewID() string {
	c.lastID++
	return fmt.Sprint(c.lastID)
}

func (c *clientConnection) isLoopEnded() (bool, <-chan InvokeResult, <-chan error) {
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

func (c *clientConnection) onConnected(hubConnection) {}

func (c *clientConnection) onDisconnected(hubConnection) {}

func (c *clientConnection) invocationTarget(hubConnection) interface{} {
	return c.receiver
}

func (c *clientConnection) allowReconnect() bool {
	return false // Servers don't care?
}

func (c *clientConnection) prefixLoggers() (info StructuredLogger, dbg StructuredLogger) {
	if c.receiver == nil {
		return log.WithPrefix(c.info, "ts", log.DefaultTimestampUTC, "class", "Client"),
			log.WithPrefix(c.dbg, "ts", log.DefaultTimestampUTC, "class", "Client")
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
			"hub", t),
		log.WithPrefix(c.dbg, "ts", log.DefaultTimestampUTC,
			"class", "Client",
			"hub", t)
}

func (c *clientConnection) processHandshake() (HubProtocol, error) {
	const request = "{\"Protocol\":\"json\",\"Version\":1}\u001e"
	_, err := c.conn.Write([]byte(request))
	if err != nil {
		fmt.Println(err)
	}
	var buf bytes.Buffer
	data := make([]byte, 1<<12)
	for {
		var n int
		if n, err = c.conn.Read(data); err != nil {
			break
		} else {
			buf.Write(data[:n])
			var rawHandshake []byte
			if rawHandshake, err = parseTextMessageFormat(&buf); err != nil {
				// Partial message, read more data
				buf.Write(data[:n])
			} else {
				response := handshakeResponse{}
				if err = json.Unmarshal(rawHandshake, &response); err != nil {
					// Malformed handshake
					break
				}
				fmt.Println(response.Error)
				break
			}
		}
	}
	return &JSONHubProtocol{dbg: log.NewLogfmtLogger(os.Stderr)}, nil
}
