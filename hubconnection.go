package signalr

import (
	"bytes"
	"context"
	"errors"
	"sync"
)

type hubConnection interface {
	Start()
	IsConnected() bool
	ConnectionID() string
	Receive() (interface{}, error)
	SendInvocation(id string, target string, args []interface{}) (invocationMessage, error)
	StreamItem(id string, item interface{}) (streamItemMessage, error)
	Completion(id string, result interface{}, error string) (completionMessage, error)
	Close(error string, allowReconnect bool) (closeMessage, error)
	Ping() (hubMessage, error)
	Items() *sync.Map
	Abort()
	Aborted() <-chan error
}

func newHubConnection(parentContext context.Context, connection Connection, protocol HubProtocol, maximumReceiveMessageSize uint) hubConnection {
	return &defaultHubConnection{
		protocol:                  protocol,
		connection:                connection,
		maximumReceiveMessageSize: maximumReceiveMessageSize,
		items:                     &sync.Map{},
		context:                   parentContext,
		abortChans:                make([]chan error, 0),
	}
}

type defaultHubConnection struct {
	protocol                  HubProtocol
	mx                        sync.Mutex
	connected                 bool
	abortChans                []chan error
	connection                Connection
	maximumReceiveMessageSize uint
	items                     *sync.Map
	context                   context.Context
}

func (c *defaultHubConnection) Items() *sync.Map {
	return c.items
}

func (c *defaultHubConnection) Start() {
	defer c.mx.Unlock()
	c.mx.Lock()
	c.connected = true
}

func (c *defaultHubConnection) IsConnected() bool {
	defer c.mx.Unlock()
	c.mx.Lock()
	return c.connected
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
	defer c.mx.Unlock()
	c.mx.Lock()
	if c.connected {
		err := errors.New("connection aborted from hub")
		for _, ch := range c.abortChans {
			ch <- err
		}
		c.connected = false
	}
}

func (c *defaultHubConnection) Aborted() <-chan error {
	defer c.mx.Unlock()
	c.mx.Lock()
	ch := make(chan error, 1)
	c.abortChans = append(c.abortChans, ch)
	return ch
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

func (c *defaultHubConnection) SendInvocation(id string, target string, args []interface{}) (invocationMessage, error) {
	var invocationMessage = invocationMessage{
		Type:         1,
		InvocationID: id,
		Target:       target,
		Arguments:    args,
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
	_, isCloseMsg := message.(closeMessage)
	if !c.IsConnected() &&
		// Allow sending closeMessage when not connected
		!isCloseMsg {
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
