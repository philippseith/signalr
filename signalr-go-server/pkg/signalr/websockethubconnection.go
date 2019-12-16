package signalr

import (
	"golang.org/x/net/websocket"
	"sync/atomic"
)

type webSocketHubConnection struct {
	ws           *websocket.Conn
	protocol     HubProtocol
	connectionID string
	connected    int32
}

func (w *webSocketHubConnection) isConnected() bool {
	return atomic.LoadInt32(&w.connected) == 1
}

func (w *webSocketHubConnection) getConnectionID() string {
	return w.connectionID
}

func (w *webSocketHubConnection) sendInvocation(target string, args []interface{}) {
	var invocationMessage = sendOnlyHubInvocationMessage{
		Type:      1,
		Target:    target,
		Arguments: args,
	}

	w.protocol.WriteMessage(invocationMessage, w.ws)
}

func (w *webSocketHubConnection) ping() {
	var pingMessage = hubMessage{
		Type: 6,
	}

	w.protocol.WriteMessage(pingMessage, w.ws)
}

func (w *webSocketHubConnection) start() {
	atomic.CompareAndSwapInt32(&w.connected, 0, 1)
}

func (w *webSocketHubConnection) completion(id string, result interface{}, error string) {
	var completionMessage = completionMessage{
		Type:         3,
		InvocationID: id,
		Result:       result,
		Error:        error,
	}

	w.protocol.WriteMessage(completionMessage, w.ws)
}

func (w *webSocketHubConnection) streamItem(id string, item interface{}) {
	var streamItemMessage = streamItemMessage{
		Type:         3,
		InvocationID: id,
		Item:         item,
	}

	w.protocol.WriteMessage(streamItemMessage, w.ws)
}

func (w *webSocketHubConnection) close(error string) {
	atomic.StoreInt32(&w.connected, 0)

	var closeMessage = closeMessage{
		Type:           6,
		Error:          error,
		AllowReconnect: true,
	}

	w.protocol.WriteMessage(closeMessage, w.ws)
}

