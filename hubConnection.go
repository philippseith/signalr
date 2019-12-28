package signalr

import (
	"bytes"
	"io"
	"sync/atomic"
)

type HubConnection interface {
	Start()
	IsConnected() bool
	Close(error string)
	GetConnectionID() string
	Receive() (interface{}, error)
	SendInvocation(target string, args []interface{})
	StreamItem(id string, item interface{})
	Completion(id string, result interface{}, error string)
	Ping()
}

type Connection interface {
	io.Reader
	io.Writer
}

func NewHubConnection(connection Connection, connectionID string, protocol HubProtocol) HubConnection {
	return &defaultHubConnection{
		Protocol:     protocol,
		ConnectionID: connectionID,
		Writer:       connection,
		Reader:       connection,
	}
}

type defaultHubConnection struct {
	Protocol     HubProtocol
	ConnectionID string
	Connected    int32
	Writer       io.Writer
	Reader       io.Reader
}

func (c *defaultHubConnection) Start() {
	atomic.CompareAndSwapInt32(&c.Connected, 0, 1)
}


func (c *defaultHubConnection) IsConnected() bool {
	return atomic.LoadInt32(&c.Connected) == 1
}

func (c *defaultHubConnection) Close(error string) {
	atomic.StoreInt32(&c.Connected, 0)

	var closeMessage = closeMessage{
		Type:           7,
		Error:          error,
		AllowReconnect: true,
	}
	c.Protocol.WriteMessage(closeMessage, c.Writer)
}

func (c *defaultHubConnection) GetConnectionID() string {
	return c.ConnectionID
}

func (c *defaultHubConnection) SendInvocation(target string, args []interface{}) {
	var invocationMessage = sendOnlyHubInvocationMessage{
		Type:      1,
		Target:    target,
		Arguments: args,
	}

	c.Protocol.WriteMessage(invocationMessage, c.Writer)
}

func (c *defaultHubConnection) Ping() {
	var pingMessage = hubMessage{
		Type: 6,
	}

	c.Protocol.WriteMessage(pingMessage, c.Writer)
}

func (c *defaultHubConnection) Receive() (interface{}, error) {
	var buf bytes.Buffer
	var data = make([]byte, 1<<15) // 32K
	var n int
	for {
		if message, complete, err := c.Protocol.ReadMessage(&buf); !complete {
			// Partial message, need more data
			// ReadMessage read data out of the buf, so its gone there: refill
			buf.Write(data[:n])
			if n, err = c.Reader.Read(data); err != nil {
				return nil, err
			} else {
				buf.Write(data[:n])
			}
		} else {
			return message, err
		}
	}
}

func (c *defaultHubConnection) Completion(id string, result interface{}, error string) {
	var completionMessage = completionMessage{
		Type:         3,
		InvocationID: id,
		Result:       result,
		Error:        error,
	}

	c.Protocol.WriteMessage(completionMessage, c.Writer)
}

func (c *defaultHubConnection) StreamItem(id string, item interface{}) {
	var streamItemMessage = streamItemMessage{
		Type:         2,
		InvocationID: id,
		Item:         item,
	}

	c.Protocol.WriteMessage(streamItemMessage, c.Writer)
}

