package signalr

import (
	"bytes"
	"context"
	"sync"
	"sync/atomic"
)

type hubConnection interface {
	Start()
	IsConnected() bool
	ConnectionID() string
	Receive() (interface{}, error)
	SendInvocation(target string, args ...interface{}) (sendOnlyHubInvocationMessage, error)
	StreamItem(id string, item interface{}) (streamItemMessage, error)
	Completion(id string, result interface{}, error string) (completionMessage, error)
	Close(error string, allowReconnect bool) (closeMessage, error)
	Ping() (hubMessage, error)
	Items() *sync.Map
	Context() context.Context
	Abort()
}

func newHubConnection(connection Connection, protocol HubProtocol, maximumReceiveMessageSize int, parentContext context.Context) hubConnection {
	ctx, abort := context.WithCancel(parentContext)
	return &defaultHubConnection{
		Protocol:                  protocol,
		Connection:                connection,
		maximumReceiveMessageSize: maximumReceiveMessageSize,
		items:                     &sync.Map{},
		context:                   ctx,
		abort:                     abort,
	}
}

type defaultHubConnection struct {
	Protocol                  HubProtocol
	Connected                 int32
	Connection                Connection
	maximumReceiveMessageSize int
	items                     *sync.Map
	context                   context.Context
	abort                     context.CancelFunc
}

func (c *defaultHubConnection) Items() *sync.Map {
	return c.items
}

func (c *defaultHubConnection) Start() {
	atomic.SwapInt32(&c.Connected, 1)
}

func (c *defaultHubConnection) IsConnected() bool {
	return atomic.LoadInt32(&c.Connected) == 1
}

func (c *defaultHubConnection) Close(error string, allowReconnect bool) (closeMessage, error) {
	defer c.Abort()
	var closeMessage = closeMessage{
		Type:           7,
		Error:          error,
		AllowReconnect: allowReconnect,
	}
	return closeMessage, c.writeMessage(closeMessage)
}

func (c *defaultHubConnection) ConnectionID() string {
	return c.Connection.ConnectionID()
}

func (c *defaultHubConnection) Context() context.Context {
	return c.context
}

func (c *defaultHubConnection) Abort() {
	c.abort()
	atomic.SwapInt32(&c.Connected, 0)
}

func (c *defaultHubConnection) Receive() (interface{}, error) {
	if !c.IsConnected() {
		return nil, c.context.Err()
	}
	m := make(chan interface{}, 1)
	e := make(chan error, 1)
	go func() {
		var buf bytes.Buffer
		var data = make([]byte, c.maximumReceiveMessageSize)
		var n int
		for {
			if message, complete, err := c.Protocol.ReadMessage(&buf); !complete {
				// Partial message, need more data
				// ReadMessage read data out of the buf, so its gone there: refill
				buf.Write(data[:n])
				if n, err = c.Connection.Read(data); err == nil {
					buf.Write(data[:n])
				} else {
					m <- nil
					e <- err
					break
				}
			} else {
				m <- message
				e <- err
				break
			}
		}
	}()
	select {
	case <-c.context.Done():
		// Wait for ReadMessage to return
		<-e
		return nil, c.context.Err()
	case err := <-e:
		return <-m, err
	}
}

func (c *defaultHubConnection) SendInvocation(target string, args ...interface{}) (sendOnlyHubInvocationMessage, error) {
	var invocationMessage = sendOnlyHubInvocationMessage{
		Type:      1,
		Target:    target,
		Arguments: args,
	}
	return invocationMessage, c.writeMessage(invocationMessage)
}

func (c *defaultHubConnection) StreamItem(id string, item interface{}) (streamItemMessage, error) {
	var streamItemMessage = streamItemMessage{
		Type:         2,
		InvocationID: id,
		Item:         item,
	}
	return streamItemMessage, c.writeMessage(streamItemMessage)
}

func (c *defaultHubConnection) Completion(id string, result interface{}, error string) (completionMessage, error) {
	var completionMessage = completionMessage{
		Type:         3,
		InvocationID: id,
		Result:       result,
		Error:        error,
	}
	return completionMessage, c.writeMessage(completionMessage)
}

func (c *defaultHubConnection) Ping() (hubMessage, error) {
	var pingMessage = hubMessage{
		Type: 6,
	}
	return pingMessage, c.writeMessage(pingMessage)
}

func (c *defaultHubConnection) writeMessage(message interface{}) error {
	if !c.IsConnected() {
		return c.context.Err()
	}
	e := make(chan error, 1)
	go func() { e <- c.Protocol.WriteMessage(message, c.Connection) }()
	select {
	case <-c.context.Done():
		// Wait for WriteMessage to return
		<-e
		return c.context.Err()
	case err := <-e:
		return err
	}
}
