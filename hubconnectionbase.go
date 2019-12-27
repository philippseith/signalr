package signalr

import (
	"bytes"
	"io"
	"sync/atomic"
)

type HubConnectionBase struct {
	Protocol     HubProtocol
	ConnectionID string
	Connected    int32
	Writer       io.Writer
	Reader       io.Reader
}

func (c *HubConnectionBase) IsConnected() bool {
	return atomic.LoadInt32(&c.Connected) == 1
}

func (c *HubConnectionBase) GetConnectionID() string {
	return c.ConnectionID
}

func (c *HubConnectionBase) SendInvocation(target string, args []interface{}) {
	var invocationMessage = sendOnlyHubInvocationMessage{
		Type:      1,
		Target:    target,
		Arguments: args,
	}

	c.Protocol.WriteMessage(invocationMessage, c.Writer)
}

func (c *HubConnectionBase) Ping() {
	var pingMessage = hubMessage{
		Type: 6,
	}

	c.Protocol.WriteMessage(pingMessage, c.Writer)
}

func (c *HubConnectionBase) start() {
	atomic.CompareAndSwapInt32(&c.Connected, 0, 1)
}

func (c *HubConnectionBase) Receive() (interface{}, error) {
	var buf bytes.Buffer
	var data = make([]byte, 1 << 15) // 32K
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

func (c *HubConnectionBase) Completion(id string, result interface{}, error string) {
	var completionMessage = completionMessage{
		Type:         3,
		InvocationID: id,
		Result:       result,
		Error:        error,
	}

	c.Protocol.WriteMessage(completionMessage, c.Writer)
}

func (c *HubConnectionBase) StreamItem(id string, item interface{}) {
	var streamItemMessage = streamItemMessage{
		Type:         2,
		InvocationID: id,
		Item:         item,
	}

	c.Protocol.WriteMessage(streamItemMessage, c.Writer)
}

func (c *HubConnectionBase) close(error string) {
	atomic.StoreInt32(&c.Connected, 0)

	var closeMessage = closeMessage{
		Type:           7,
		Error:          error,
		AllowReconnect: true,
	}

	c.Protocol.WriteMessage(closeMessage, c.Writer)
}

