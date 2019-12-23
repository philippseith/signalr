package signalr

import (
	"bytes"
	"golang.org/x/net/websocket"
	"io"
	"sync/atomic"
)

type baseHubConnection struct {
	protocol     HubProtocol
	connectionID string
	connected    int32
	writer       io.Writer
	reader			 io.Reader
}

func (w *baseHubConnection) isConnected() bool {
	return atomic.LoadInt32(&w.connected) == 1
}

func (w *baseHubConnection) getConnectionID() string {
	return w.connectionID
}

func (w *baseHubConnection) sendInvocation(target string, args []interface{}) {
	var invocationMessage = sendOnlyHubInvocationMessage{
		Type:      1,
		Target:    target,
		Arguments: args,
	}

	w.protocol.WriteMessage(invocationMessage, w.writer)
}

func (w *baseHubConnection) ping() {
	var pingMessage = hubMessage{
		Type: 6,
	}

	w.protocol.WriteMessage(pingMessage, w.writer)
}

func (w *baseHubConnection) start() {
	atomic.CompareAndSwapInt32(&w.connected, 0, 1)
}

func (w *baseHubConnection) completion(id string, result interface{}, error string) {
	var completionMessage = completionMessage{
		Type:         3,
		InvocationID: id,
		Result:       result,
		Error:        error,
	}

	w.protocol.WriteMessage(completionMessage, w.writer)
}

func (w *baseHubConnection) streamItem(id string, item interface{}) {
	var streamItemMessage = streamItemMessage{
		Type:         2,
		InvocationID: id,
		Item:         item,
	}

	w.protocol.WriteMessage(streamItemMessage, w.writer)
}

func (w *baseHubConnection) close(error string) {
	atomic.StoreInt32(&w.connected, 0)

	var closeMessage = closeMessage{
		Type:           7,
		Error:          error,
		AllowReconnect: true,
	}

	w.protocol.WriteMessage(closeMessage, w.writer)
}

