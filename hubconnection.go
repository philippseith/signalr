package signalr

import (
	"bytes"
	"fmt"
	"github.com/go-kit/kit/log"
	"sync/atomic"
)

type hubConnection interface {
	Start()
	IsConnected() bool
	Close(error string)
	GetConnectionID() string
	Receive() (interface{}, error)
	SendInvocation(target string, args []interface{})
	StreamItem(id string, item interface{})
	Completion(id string, result interface{}, error string)
	Ping()
	Items() map[string]interface{}
}

func newHubConnection(connection Connection, protocol HubProtocol, info log.Logger, debug log.Logger) hubConnection {
	return &defaultHubConnection{
		Protocol:   protocol,
		Connection: connection,
		items:      make(map[string]interface{}),
		info: info,
		debug: debug,
	}
}

type defaultHubConnection struct {
	Protocol   HubProtocol
	Connected  int32
	Connection Connection
	items      map[string]interface{}
	info log.Logger
	debug log.Logger
}

func (c *defaultHubConnection) Items() map[string]interface{} {
	return c.items
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
	if err := c.Protocol.WriteMessage(closeMessage, c.Connection); err != nil {
		fmt.Printf("cannot close connection %v: %v", c.GetConnectionID(), err)
	}
}

func (c *defaultHubConnection) GetConnectionID() string {
	return c.Connection.ConnectionID()
}

func (c *defaultHubConnection) SendInvocation(target string, args []interface{}) {
	var invocationMessage = sendOnlyHubInvocationMessage{
		Type:      1,
		Target:    target,
		Arguments: args,
	}

	if err := c.Protocol.WriteMessage(invocationMessage, c.Connection); err != nil {
		fmt.Printf("cannot send invocation %v %v over connection %v: %v", target, args, c.GetConnectionID(), err)
	}
}

func (c *defaultHubConnection) Ping() {
	var pingMessage = hubMessage{
		Type: 6,
	}

	if err := c.Protocol.WriteMessage(pingMessage, c.Connection); err != nil {
		fmt.Printf("cannot ping over connection %v: %v", c.GetConnectionID(), err)
	}
}

func (c *defaultHubConnection) Receive() (interface{}, error) {
	var buf bytes.Buffer
	var data = make([]byte, 1<<12) // 4K
	var n int
	for {
		if message, complete, err := c.Protocol.ReadMessage(&buf); !complete {
			// Partial message, need more data
			// ReadMessage read data out of the buf, so its gone there: refill
			buf.Write(data[:n])
			if n, err = c.Connection.Read(data); err == nil {
				buf.Write(data[:n])
			} else {
				return nil, err
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

	if err := c.Protocol.WriteMessage(completionMessage, c.Connection); err != nil {
		fmt.Printf("cannot send completion for invocation %v over connection %v: %v", id, c.GetConnectionID(), err)
	}
}

func (c *defaultHubConnection) StreamItem(id string, item interface{}) {
	var streamItemMessage = streamItemMessage{
		Type:         2,
		InvocationID: id,
		Item:         item,
	}

	if err := c.Protocol.WriteMessage(streamItemMessage, c.Connection); err != nil {
		fmt.Printf("cannot send stream item for invocation %v over connection %v: %v", id, c.GetConnectionID(), err)
	}
}
