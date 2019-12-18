package signalr

import (
	"bytes"
	"fmt"
	"net/http"
	"reflect"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/websocket"
)

// MapHub used to register a SignalR Hub with the specified ServeMux
func MapHub(mux *http.ServeMux, path string, hubPrototype HubInterface) {
	lifetimeManager := defaultHubLifetimeManager{}
	defaultHubClients := defaultHubClients{
		lifetimeManager: &lifetimeManager,
		allCache:        allClientProxy{lifetimeManager: &lifetimeManager},
	}

	groupManager := defaultGroupManager{
		lifetimeManager: &lifetimeManager,
	}

	hubType := reflect.TypeOf(hubPrototype)

	mux.HandleFunc(fmt.Sprintf("%s/negotiate", path), negotiateHandler)
	mux.Handle(path, websocket.Handler(func(ws *websocket.Conn) {
		connectionID := ws.Request().URL.Query().Get("id")

		if len(connectionID) == 0 {
			// Support websocket connection without negotiate
			connectionID = getConnectionID()
		}

		// Copy hubPrototype
		hub := reflect.New(reflect.ValueOf(hubPrototype).Elem().Type()).Interface().(HubInterface)

		hubContext := &defaultHubContext{
			clients: &contextHubClients{
				defaultHubClients: &defaultHubClients,
				connectionID:      connectionID,
			},
			groups: &groupManager,
		}

		hub.Initialize(hubContext)

		hubInfo := &hubInfo{
			hub:             hub,
			lifetimeManager: &lifetimeManager,
			methods:         make(map[string]reflect.Value),
		}

		hubValue := reflect.ValueOf(hub)
		for i := 0; i < hubType.NumMethod(); i++ {
			m := hubType.Method(i)
			hubInfo.methods[strings.ToLower(m.Name)] = hubValue.Method(i)
		}

		hubConnectionHandler(connectionID, ws, hubInfo)
	}))
}

// Implementation

type hubInfo struct {
	hub             HubInterface
	lifetimeManager HubLifetimeManager
	methods         map[string]reflect.Value
}

func hubConnectionHandler(connectionID string, ws *websocket.Conn, hubInfo *hubInfo) {
	var err error
	var buf bytes.Buffer
	protocol, err := processHandshake(ws, &buf)

	if err != nil {
		fmt.Println(err)
		return
	}

	conn := webSocketHubConnection{protocol: protocol, connectionID: connectionID, ws: ws}
	conn.start()

	streamer := newStreamer(conn)
	streamClient := newStreamClient()

	hubInfo.lifetimeManager.OnConnected(&conn)
	hubInfo.hub.OnConnected(connectionID)

	// Start sending pings to the client
	pings := startPingClientLoop(conn)

	for conn.isConnected() {
		for {
			message, err := protocol.ReadMessage(&buf)

			if err != nil {
				// Partial message, need more data
				break
			}

			switch message.(type) {
			case invocationMessage:
				invocation := message.(invocationMessage)
				// Dispatch invocation here
				if method, ok := hubInfo.methods[strings.ToLower(invocation.Target)]; !ok {
					// Unable to find the method
					conn.completion(invocation.InvocationID, nil, fmt.Sprintf("Unknown method %s", invocation.Target))
				} else if in, clientStreaming, err := buildMethodArguments(method, invocation, streamClient, protocol); err != nil {
					// argument build failed
					conn.completion(invocation.InvocationID, nil, err.Error())
				} else if clientStreaming {
					// let the receiving method run idependently
					go func() {
						defer func() {
							if err := recover(); err != nil {
								conn.completion(invocation.InvocationID, nil, fmt.Sprintf("%v\n%v", err, string(debug.Stack())))
							}
						}()
						method.Call(in)
					}()
				} else {
					result := func() []reflect.Value {
						defer func() {
							if err := recover(); err != nil {
								conn.completion(invocation.InvocationID, nil, fmt.Sprintf("%v\n%v", err, string(debug.Stack())))
							}
						}()
						return method.Call(in)
					}()
					returnInvocationResult(conn, invocation, streamer, result)
				}
			case cancelInvocationMessage:
				streamer.Stop(message.(cancelInvocationMessage).InvocationID)
			case streamItemMessage:
				streamClient.receiveStreamItem(message.(streamItemMessage))
			case completionMessage:
				streamClient.receiveCompletionItem(message.(completionMessage))
			case hubMessage:
				// Ping
			}
		}

		var data []byte
		if err = websocket.Message.Receive(ws, &data); err != nil {
			fmt.Println(err)
			break
		}
		// Main message loop
		fmt.Println("Message received " + string(data))
		buf.Write(data)
	}

	hubInfo.hub.OnDisconnected(connectionID)
	hubInfo.lifetimeManager.OnDisconnected(&conn)
	conn.close("")

	// Wait for pings to complete
	pings.Wait()
}

func returnInvocationResult(conn webSocketHubConnection, invocation invocationMessage, streamer *streamer, result []reflect.Value) {
	// if the hub method returns a chan, it should be considered asynchronous or source for a stream
	if len(result) == 1 && result[0].Kind() == reflect.Chan {
		switch invocation.Type {
		// Simple invocation
		case 1:
			go func() {
				// Recv might block, so run continue in a goroutine
				if chanResult, ok := result[0].Recv(); ok {
					invokeConnection(conn, invocation, completion, []reflect.Value{chanResult})
				} else {
					conn.completion(invocation.InvocationID, nil, "go channel closed")
				}
			}()
		// StreamInvocation
		case 4:
			streamer.Start(invocation.InvocationID, result[0])
		}
	} else {
		switch invocation.Type {
		// Simple invocation
		case 1:
			invokeConnection(conn, invocation, completion, result)
		case 4:
			// Stream invocation of method with no stream result.
			// Return a single StreamItem and an empty completion
			invokeConnection(conn, invocation, streamItem, result)
			conn.completion(invocation.InvocationID, nil, "")
		}
	}
}

func buildMethodArguments(method reflect.Value, invocation invocationMessage,
	streamClient *streamClient, protocol HubProtocol) (arguments []reflect.Value, clientStreaming bool, err error) {
	arguments = make([]reflect.Value, method.Type().NumIn())
	chanCount := 0
	for i := 0; i < method.Type().NumIn(); i++ {
		t := method.Type().In(i)
		// Is it a channel for client streaming?
		if arg, clientStreaming, err := streamClient.buildChannelArgument(invocation, t, chanCount); err != nil {
			// it is, but channel count in invocation and method mismatch
			return nil, false, err
		} else if clientStreaming {
			// it is
			chanCount++
			arguments[i] = arg
		} else {
			// it is not, so do the normal thing
			arg := reflect.New(t)
			if err := protocol.UnmarshalArgument(invocation.Arguments[i-chanCount], arg.Interface()); err != nil {
				return arguments, chanCount > 0, err
			}
			arguments[i] = arg.Elem()
		}
	}
	return arguments, chanCount > 0, nil
}

func startPingClientLoop(conn webSocketHubConnection) sync.WaitGroup {
	var waitgroup sync.WaitGroup
	waitgroup.Add(1)
	go func(waitGroup *sync.WaitGroup, conn hubConnection) {
		defer waitGroup.Done()

		for conn.isConnected() {
			conn.ping()
			time.Sleep(5 * time.Second)
		}
	}(&waitgroup, &conn)
	return waitgroup
}

type connFunc func(conn webSocketHubConnection, invocation invocationMessage, value interface{})

func completion(conn webSocketHubConnection, invocation invocationMessage, value interface{}) {
	conn.completion(invocation.InvocationID, value, "")
}

func streamItem(conn webSocketHubConnection, invocation invocationMessage, value interface{}) {
	conn.streamItem(invocation.InvocationID, value)
}

func invokeConnection(conn webSocketHubConnection, invocation invocationMessage, connFunc connFunc, result []reflect.Value) {
	values := make([]interface{}, len(result))
	for i, rv := range result {
		values[i] = rv.Interface()
	}
	switch len(result) {
	case 0:
		conn.completion(invocation.InvocationID, nil, "")
	case 1:
		connFunc(conn, invocation, values[0])
	default:
		connFunc(conn, invocation, values)
	}
}



