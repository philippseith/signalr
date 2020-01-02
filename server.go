package signalr

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

type Server struct {
	hub               HubInterface
	lifetimeManager   HubLifetimeManager
	defaultHubClients defaultHubClients
	groupManager      GroupManager
}

func NewServer(hub HubInterface) *Server {
	lifetimeManager := defaultHubLifetimeManager{}
	return &Server{
		hub:             hub,
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

func (s *Server) messageLoop(conn Connection) {
	if protocol, err := processHandshake(conn); err != nil {
		fmt.Println(err)
	} else {
		hubConn := newHubConnection(conn, protocol)
		// start sending pings to the client
		pings := startPingClientLoop(hubConn)
		hubConn.Start()
		// Process messages
		streamer := newStreamer(hubConn)
		streamClient := newStreamClient()
		hubInfo := s.newHubInfo()
		hubInfo.lifetimeManager.OnConnected(hubConn)
		hubInfo.hub.OnConnected(hubConn.GetConnectionID())

		for hubConn.IsConnected() {
			if message, err := hubConn.Receive(); err != nil {
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
						hubConn.Completion(invocation.InvocationID, nil, fmt.Sprintf("Unknown method %s", invocation.Target))
					} else if in, clientStreaming, err := buildMethodArguments(method, invocation, streamClient, protocol); err != nil {
						// argument build failed
						hubConn.Completion(invocation.InvocationID, nil, err.Error())
					} else if clientStreaming {
						// let the receiving method run idependently
						go func() {
							defer func() {
								if err := recover(); err != nil {
									hubConn.Completion(invocation.InvocationID, nil, fmt.Sprintf("%v\n%v", err, string(debug.Stack())))
								}
							}()
							method.Call(in)
						}()
					} else {
						result := func() []reflect.Value {
							defer func() {
								if err := recover(); err != nil {
									hubConn.Completion(invocation.InvocationID, nil, fmt.Sprintf("%v\n%v", err, string(debug.Stack())))
								}
							}()
							return method.Call(in)
						}()
						returnInvocationResult(hubConn, invocation, streamer, result)
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
		hubInfo.hub.OnDisconnected(hubConn.GetConnectionID())
		hubInfo.lifetimeManager.OnDisconnected(hubConn)
		hubConn.Close("")
		// Wait for pings to complete
		pings.Wait()
	}
}

func startPingClientLoop(conn hubConnection) *sync.WaitGroup {
	var waitgroup sync.WaitGroup
	waitgroup.Add(1)
	go func(waitGroup *sync.WaitGroup, conn hubConnection) {
		defer waitGroup.Done()

		for conn.IsConnected() {
			conn.Ping()
			time.Sleep(5 * time.Second)
		}
	}(&waitgroup, conn)
	return &waitgroup
}

type hubInfo struct {
	hub             HubInterface
	lifetimeManager HubLifetimeManager
	methods         map[string]reflect.Value
}

func (s *Server) newHubInfo() *hubInfo {

	s.hub.Initialize(&defaultHubContext{
		clients: &s.defaultHubClients,
		groups:  s.groupManager,
	})

	hubInfo := &hubInfo{
		hub:             s.hub,
		lifetimeManager: s.lifetimeManager,
		methods:         make(map[string]reflect.Value),
	}

	hubType := reflect.TypeOf(s.hub)
	hubValue := reflect.ValueOf(s.hub)
	for i := 0; i < hubType.NumMethod(); i++ {
		m := hubType.Method(i)
		hubInfo.methods[strings.ToLower(m.Name)] = hubValue.Method(i)
	}
	return hubInfo
}

func returnInvocationResult(conn hubConnection, invocation invocationMessage, streamer *streamer, result []reflect.Value) {
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

type connFunc func(conn hubConnection, invocation invocationMessage, value interface{})

func completion(conn hubConnection, invocation invocationMessage, value interface{}) {
	conn.Completion(invocation.InvocationID, value, "")
}

func streamItem(conn hubConnection, invocation invocationMessage, value interface{}) {
	conn.StreamItem(invocation.InvocationID, value)
}

func invokeConnection(conn hubConnection, invocation invocationMessage, connFunc connFunc, result []reflect.Value) {
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

func processHandshake(conn Connection) (HubProtocol, error) {
	var err error
	var protocol HubProtocol
	var ok bool
	const handshakeResponse = "{}\u001e"
	const errorHandshakeResponse = "{\"error\":\"%s\"}\u001e"

	// TODO 5 seconds to process the handshake
	// ws.SetReadDeadline(time.Now().Add(5 * time.Second))

	var buf bytes.Buffer
	data := make([]byte, 1<<12)
	for {
		n, err := conn.Read(data)
		if err != nil {
			break
		}

		buf.Write(data[:n])

		rawHandshake, err := parseTextMessageFormat(&buf)

		if err != nil {
			// Partial message, read more data
			continue
		}

		fmt.Println("Handshake received")

		request := handshakeRequest{}
		err = json.Unmarshal(rawHandshake, &request)

		if err != nil {
			// Malformed handshake
			break
		}

		protocol, ok = protocolMap[request.Protocol]

		if ok {
			// Send the handshake response
			_, err = conn.Write([]byte(handshakeResponse))
		} else {
			// Protocol not supported
			fmt.Printf("\"%s\" is the only supported Protocol\n", request.Protocol)
			_, err = conn.Write([]byte(fmt.Sprintf(errorHandshakeResponse, fmt.Sprintf("Protocol \"%s\" not supported", request.Protocol))))
		}
		break
	}

	// TODO Disable the timeout (either we already timeout out or)
	//ws.SetReadDeadline(time.Time{})

	return protocol, err
}

var protocolMap = map[string]HubProtocol{
	"json": &JsonHubProtocol{},
}

type availableTransport struct {
	Transport       string   `json:"transport"`
	TransferFormats []string `json:"transferFormats"`
}

type negotiateResponse struct {
	ConnectionID        string               `json:"connectionId"`
	AvailableTransports []availableTransport `json:"availableTransports"`
}
