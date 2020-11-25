package signalr

import (
	"bytes"
	"context"
	"github.com/rotisserie/eris"
	"io"
	"sync"
	"time"
)

// hubConnection is used by HubContext, Server and Client to realize the external API.
// hubConnection uses a transport connection (of type Connection) and a hubProtocol to send and receive SignalR messages.
type hubConnection interface {
	ConnectionID() string
	Receive() <-chan receiveResult
	SendInvocation(id string, target string, args []interface{}) error
	SendStreamInvocation(id string, target string, args []interface{}, streamIds []string) error
	StreamItem(id string, item interface{}) error
	Completion(id string, result interface{}, error string) error
	Close(error string, allowReconnect bool) error
	Ping() error
	LastWriteStamp() time.Time
	Items() *sync.Map
	Context() context.Context
	Abort()
}

type receiveResult struct {
	message interface{}
	err     error
}

func newHubConnection(connection Connection, protocol hubProtocol, maximumReceiveMessageSize uint, info StructuredLogger) hubConnection {
	ctx, cancelFunc := context.WithCancel(connection.Context())
	c := &defaultHubConnection{
		ctx:                       ctx,
		cancelFunc:                cancelFunc,
		protocol:                  protocol,
		mx:                        sync.Mutex{},
		connection:                connection,
		maximumReceiveMessageSize: maximumReceiveMessageSize,
		items:                     &sync.Map{},
		info:                      info,
	}
	return c
}

type defaultHubConnection struct {
	ctx                       context.Context
	cancelFunc                context.CancelFunc
	protocol                  hubProtocol
	mx                        sync.Mutex
	connection                Connection
	maximumReceiveMessageSize uint
	items                     *sync.Map
	lastWriteStamp            time.Time
	info                      StructuredLogger
}

func (c *defaultHubConnection) Items() *sync.Map {
	return c.items
}

func (c *defaultHubConnection) Close(errorText string, allowReconnect bool) error {
	var closeMessage = closeMessage{
		Type:           7,
		Error:          errorText,
		AllowReconnect: allowReconnect,
	}
	return c.protocol.WriteMessage(closeMessage, c.connection)
}

func (c *defaultHubConnection) ConnectionID() string {
	return c.connection.ConnectionID()
}

func (c *defaultHubConnection) Context() context.Context {
	return c.ctx
}

func (c *defaultHubConnection) Abort() {
	c.cancelFunc()
}

func (c *defaultHubConnection) Receive() <-chan receiveResult {
	recvChan := make(chan receiveResult, 20)
	// the pipe connects the goroutine which reads from the connection and the goroutine which parses the read data
	reader, writer := io.Pipe()
	p := make([]byte, c.maximumReceiveMessageSize)
	go func(ctx context.Context, connection io.Reader, writer io.Writer, recvChan chan receiveResult) {
		for {
			if ctx.Err() != nil {
				break
			}
			n, err := connection.Read(p)
			if err != nil &&
				ctx.Err() == nil { // if ctx.Err != nil, recvChan is closed and we shouldn't push to it
				recvChan <- receiveResult{err: err}
			}
			if ctx.Err() != nil {
				break
			}
			if n > 0 {
				_, err = writer.Write(p[:n])
				if err != nil &&
					ctx.Err() == nil { // if ctx.Err != nil, recvChan is closed and we shouldn't push to it
					recvChan <- receiveResult{err: err}
				}
			}
		}
	}(c.ctx, c.connection, writer, recvChan)
	// parse
	go func(ctx context.Context, reader io.Reader, recvChan chan receiveResult) {
		remainBuf := bytes.Buffer{}
		for {
			if ctx.Err() != nil {
				close(recvChan)
				break
			}
			messages, err := c.protocol.ParseMessages(reader, &remainBuf)
			if err != nil && ctx.Err() == nil { // if ctx.Err != nil, recvChan is closed and we shouldn't push to it
				recvChan <- receiveResult{err: err}
			}
			for _, message := range messages {
				if ctx.Err() == nil { // if ctx.Err != nil, recvChan is closed and we shouldn't push to it
					recvChan <- receiveResult{message: message}
				}
			}
		}
	}(c.ctx, reader, recvChan)
	return recvChan
}

func (c *defaultHubConnection) SendInvocation(id string, target string, args []interface{}) error {
	if args == nil {
		args = make([]interface{}, 0)
	}
	var invocationMessage = invocationMessage{
		Type:         1,
		InvocationID: id,
		Target:       target,
		Arguments:    args,
	}
	return c.writeMessage(invocationMessage)
}

func (c *defaultHubConnection) SendStreamInvocation(id string, target string, args []interface{}, streamIds []string) error {
	var invocationMessage = invocationMessage{
		Type:         4,
		InvocationID: id,
		Target:       target,
		Arguments:    args,
		StreamIds:    streamIds,
	}
	return c.writeMessage(invocationMessage)
}

func (c *defaultHubConnection) StreamItem(id string, item interface{}) error {
	var streamItemMessage = streamItemMessage{
		Type:         2,
		InvocationID: id,
		Item:         item,
	}
	return c.writeMessage(streamItemMessage)
}

func (c *defaultHubConnection) Completion(id string, result interface{}, error string) error {
	var completionMessage = completionMessage{
		Type:         3,
		InvocationID: id,
		Result:       result,
		Error:        error,
	}
	return c.writeMessage(completionMessage)
}

func (c *defaultHubConnection) Ping() error {
	var pingMessage = hubMessage{
		Type: 6,
	}
	return c.writeMessage(pingMessage)
}

func (c *defaultHubConnection) LastWriteStamp() time.Time {
	return c.lastWriteStamp
}

func (c *defaultHubConnection) writeMessage(message interface{}) error {
	c.mx.Lock()
	c.lastWriteStamp = time.Now()
	c.mx.Unlock()
	err := func() error {
		if c.ctx.Err() != nil {
			return eris.Wrap(c.ctx.Err(), "hubConnection canceled")
		}
		e := make(chan error, 1)
		go func() { e <- c.protocol.WriteMessage(message, c.connection) }()
		select {
		case <-c.ctx.Done():
			return eris.Wrap(c.ctx.Err(), "hubConnection canceled")
		case err := <-e:
			if err != nil {
				c.Abort()
			}
			return err
		}
	}()
	if err != nil {
		_ = c.info.Log(evt, msgSend, "message", fmtMsg(message), "error", err)
	}
	return err
}
