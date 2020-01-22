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
	Abort()
}

func newHubConnection(parentContext context.Context, connection Connection, protocol HubProtocol, maximumReceiveMessageSize int) hubConnection {
	return &defaultHubConnection{
		protocol:                  protocol,
		connection:                connection,
		maximumReceiveMessageSize: maximumReceiveMessageSize,
		items:                     &sync.Map{},
		context:                   parentContext,
	}
}

type defaultHubConnection struct {
	protocol                  HubProtocol
	connected                 int32
	connection                Connection
	maximumReceiveMessageSize int
	items                     *sync.Map
	context                   context.Context
}

func (c *defaultHubConnection) Items() *sync.Map {
	return c.items
}

func (c *defaultHubConnection) Start() {
	atomic.SwapInt32(&c.connected, 1)
}

func (c *defaultHubConnection) IsConnected() bool {
	return atomic.LoadInt32(&c.connected) == 1
}

func (c *defaultHubConnection) Close(error string, allowReconnect bool) (closeMessage, error) {
	var closeMessage = closeMessage{
		Type:           7,
		Error:          error,
		AllowReconnect: allowReconnect,
	}
	return closeMessage, c.writeMessage(closeMessage)
}

func (c *defaultHubConnection) ConnectionID() string {
	return c.connection.ConnectionID()
}

func (c *defaultHubConnection) Abort() {
	atomic.SwapInt32(&c.connected, 0)
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
			if message, complete, err := c.protocol.ReadMessage(&buf); !complete {
				// Partial message, need more data
				// ReadMessage read data out of the buf, so its gone there: refill
				buf.Write(data[:n])
				nc := make(chan int)
				e2 := make(chan error)
				go func() {
					if n, err = c.connection.Read(data); err == nil {
						buf.Write(data[:n])
						nc <- n
					} else {
						e2 <- err
					}
				}()
				select {
				case n = <-nc:
				case err = <-e2:
					c.Abort()
					e <- err
					m <- nil
					return
				case <-c.context.Done():
					e <- c.context.Err()
					m <- nil
					return
				}
			} else {
				m <- message
				e <- err
				return
			}
		}
	}()
	select {
	case <-c.context.Done():
		return nil, c.context.Err()
	case err := <-e:
		return <-m, err
	case message := <-m:
		return message, <-e
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
	go func() { e <- c.protocol.WriteMessage(message, c.connection) }()
	select {
	case <-c.context.Done():
		// Wait for WriteMessage to return
		<-e
		c.Abort()
		return c.context.Err()
	case err := <-e:
		if err != nil {
			c.Abort()
		}
		return err
	}
}
