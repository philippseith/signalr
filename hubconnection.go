package signalr

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"time"
)

type hubConnection interface {
	Start()
	IsConnected() bool
	ConnectionID() string
	Receive() (interface{}, error)
	SendInvocation(id string, target string, args []interface{}) error
	SendStreamInvocation(id string, target string, args []interface{}, streamIds []string) error
	StreamItem(id string, item interface{}) error
	Completion(id string, result interface{}, error string) error
	Close(error string, allowReconnect bool) error
	Ping() error
	LastWriteStamp() time.Time
	Items() *sync.Map
	Abort()
	Aborted() <-chan error
}

func newHubConnection(parentContext context.Context, connection Connection, protocol HubProtocol, maximumReceiveMessageSize uint, info StructuredLogger) hubConnection {
	return &defaultHubConnection{
		protocol:                  protocol,
		mx:                        sync.Mutex{},
		connection:                connection,
		maximumReceiveMessageSize: maximumReceiveMessageSize,
		items:                     &sync.Map{},
		context:                   parentContext,
		abortChans:                make([]chan error, 0),
		info:                      info,
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
	lastWriteStamp            time.Time
	info                      StructuredLogger
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

func (c *defaultHubConnection) Close(error string, allowReconnect bool) error {
	var closeMessage = closeMessage{
		Type:           7,
		Error:          error,
		AllowReconnect: allowReconnect,
	}
	return c.writeMessage(closeMessage)
}

func (c *defaultHubConnection) ConnectionID() string {
	return c.connection.ConnectionID()
}

func (c *defaultHubConnection) Abort() {
	c.mx.Lock()
	if c.connected {
		err := errors.New("connection aborted from hub")
		for _, ch := range c.abortChans {
			go func(ch chan error) {
				ch <- err
			}(ch)
		}
		c.connected = false
	}
	c.mx.Unlock()
}

func (c *defaultHubConnection) Aborted() <-chan error {
	defer c.mx.Unlock()
	c.mx.Lock()
	ch := make(chan error, 1)
	c.abortChans = append(c.abortChans, ch)
	return ch
}

type receiveResult struct {
	message interface{}
	err     error
}

func (c *defaultHubConnection) Receive() (interface{}, error) {
	if !c.IsConnected() {
		return nil, c.context.Err()
	}
	recvResCh := make(chan receiveResult, 1)
	go func(chan receiveResult) {
		var buf bytes.Buffer
		var data = make([]byte, c.maximumReceiveMessageSize)
		var n int
		for {
			if message, complete, err := c.protocol.ReadMessage(&buf); !complete {
				// Partial message, need more data
				// ReadMessage read data out of the buf, so its gone there: refill
				buf.Write(data[:n])
				readResCh := make(chan receiveResult, 1)
				go func(chan receiveResult) {
					n, err = c.connection.Read(data)
					if err == nil {
						buf.Write(data[:n])
					}
					readResCh <- receiveResult{n, err}
					close(readResCh)
				}(readResCh)
				select {
				case readRes := <-readResCh:
					if readRes.err != nil {
						c.Abort()
						recvResCh <- readRes
						close(recvResCh)
						return
					} else {
						n = readRes.message.(int)
					}
				case <-c.context.Done():
					recvResCh <- receiveResult{err: c.context.Err()}
					close(recvResCh)
					return
				}
			} else {
				recvResCh <- receiveResult{message, err}
				close(recvResCh)
				return
			}
		}
	}(recvResCh)
	select {
	case recvRes := <-recvResCh:
		return recvRes.message, recvRes.err
	case <-c.context.Done():
		return nil, c.context.Err()
	}
}

func (c *defaultHubConnection) SendInvocation(id string, target string, args []interface{}) error {
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
	}()
	if err != nil {
		_ = c.info.Log(evt, msgSend, "message", fmtMsg(message), "error", err)
	}
	return err
}
