package signalr

import (
	"fmt"
	"reflect"
	"runtime/debug"
	"strings"
)

type Server struct {
	hubPrototype HubInterface
	lifetimeManager HubLifetimeManager
	defaultHubClients defaultHubClients
	groupManager GroupManager
}

func NewServer(hubPrototype HubInterface) *Server {
	lifetimeManager := defaultHubLifetimeManager{}
	return &Server{
		hubPrototype:    hubPrototype,
		lifetimeManager: &lifetimeManager,
		defaultHubClients: defaultHubClients{
			lifetimeManager: &lifetimeManager,
			allCache:        allClientProxy{lifetimeManager: &lifetimeManager},
		},
		groupManager: &defaultGroupManager{
			lifetimeManager: &lifetimeManager,
		},
	}
}

func (s *Server) messageLoop(conn HubConnection, connectionID string, protocol HubProtocol) {
	streamer := newStreamer(conn)
	streamClient := newStreamClient()
	hubInfo := s.newHubInfo(connectionID)
	hubInfo.lifetimeManager.OnConnected(conn)
	hubInfo.hub.OnConnected(connectionID)

	for conn.IsConnected() {
		if message, err := conn.Receive(); err != nil {
			fmt.Println(err)
			break
		} else {
			fmt.Printf("Message received %v\n", message)
			switch message.(type) {
			case invocationMessage:
				invocation := message.(invocationMessage)
				// Dispatch invocation here
				if method, ok := hubInfo.methods[strings.ToLower(invocation.Target)]; !ok {
					// Unable to find the method
					conn.Completion(invocation.InvocationID, nil, fmt.Sprintf("Unknown method %s", invocation.Target))
				} else if in, clientStreaming, err := buildMethodArguments(method, invocation, streamClient, protocol); err != nil {
					// argument build failed
					conn.Completion(invocation.InvocationID, nil, err.Error())
				} else if clientStreaming {
					// let the receiving method run idependently
					go func() {
						defer func() {
							if err := recover(); err != nil {
								conn.Completion(invocation.InvocationID, nil, fmt.Sprintf("%v\n%v", err, string(debug.Stack())))
							}
						}()
						method.Call(in)
					}()
				} else {
					result := func() []reflect.Value {
						defer func() {
							if err := recover(); err != nil {
								conn.Completion(invocation.InvocationID, nil, fmt.Sprintf("%v\n%v", err, string(debug.Stack())))
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
	}
	hubInfo.hub.OnDisconnected(connectionID)
	hubInfo.lifetimeManager.OnDisconnected(conn)
}

type hubInfo struct {
	hub             HubInterface
	lifetimeManager HubLifetimeManager
	methods         map[string]reflect.Value
}

func (s *Server) newHubInfo(connectionID string) *hubInfo {

	// Copy hubPrototype
	hub := reflect.New(reflect.ValueOf(s.hubPrototype).Elem().Type()).Interface().(HubInterface)

	hub.Initialize(&defaultHubContext{
		clients: &contextHubClients{
			defaultHubClients: &s.defaultHubClients,
			connectionID:      connectionID,
		},
		groups: s.groupManager,
	})

	hubInfo := &hubInfo{
		hub:             hub,
		lifetimeManager: s.lifetimeManager,
		methods:         make(map[string]reflect.Value),
	}

	hubType := reflect.TypeOf(hub)
	hubValue := reflect.ValueOf(hub)
	for i := 0; i < hubType.NumMethod(); i++ {
		m := hubType.Method(i)
		hubInfo.methods[strings.ToLower(m.Name)] = hubValue.Method(i)
	}
	return hubInfo
}

func returnInvocationResult(conn HubConnection, invocation invocationMessage, streamer *streamer, result []reflect.Value) {
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
					conn.Completion(invocation.InvocationID, nil, "hub func returned closed chan")
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
			// Return a single StreamItem and an empty Completion
			invokeConnection(conn, invocation, streamItem, result)
			conn.Completion(invocation.InvocationID, nil, "")
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

type connFunc func(conn HubConnection, invocation invocationMessage, value interface{})

func completion(conn HubConnection, invocation invocationMessage, value interface{}) {
	conn.Completion(invocation.InvocationID, value, "")
}

func streamItem(conn HubConnection, invocation invocationMessage, value interface{}) {
	conn.StreamItem(invocation.InvocationID, value)
}

func invokeConnection(conn HubConnection, invocation invocationMessage, connFunc connFunc, result []reflect.Value) {
	values := make([]interface{}, len(result))
	for i, rv := range result {
		values[i] = rv.Interface()
	}
	switch len(result) {
	case 0:
		conn.Completion(invocation.InvocationID, nil, "")
	case 1:
		connFunc(conn, invocation, values[0])
	default:
		connFunc(conn, invocation, values)
	}
}


