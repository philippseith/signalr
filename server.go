package signalr

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"os"
	"reflect"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

// Server is a SignalR server for one type of hub
type Server struct {
	newHub            func() HubInterface
	lifetimeManager   HubLifetimeManager
	defaultHubClients HubClients
	groupManager      GroupManager
	info              log.Logger
	dbg               log.Logger
}

// NewServer creates a new server for one type of hub
// newHub is called each time a hub method is invoked by a client to create the transient hub instance
func NewServer(options ...func(*Server) error) (*Server, error) {
	lifetimeManager := defaultHubLifetimeManager{}
	i, d := buildInfoDebugLogger(log.NewLogfmtLogger(os.Stderr), false)
	server := &Server{
		lifetimeManager: &lifetimeManager,
		defaultHubClients: &defaultHubClients{
			lifetimeManager: &lifetimeManager,
			allCache:        allClientProxy{lifetimeManager: &lifetimeManager},
		},
		groupManager: &defaultGroupManager{
			lifetimeManager: &lifetimeManager,
		},
		info: i,
		dbg:  d,
	}
	for _, option := range options {
		if option != nil {
			if err := option(server); err != nil {
				return nil, err
			}
		}
	}
	if server.newHub == nil {
		return server, errors.New("cannot determine hub type. Neither UseHub, HubFactory or SimpleHubFactory given as option")
	}
	return server, nil
}

// Run runs the server on one connection. The same server might be run on different connections in parallel
func (s *Server) Run(conn Connection) {
	info, dbg := s.prefixLogger()
	if protocol, err := s.processHandshake(conn); err != nil {
		_ = info.Log(evt, "processHandshake", "error", err, react, "do not connect")
	} else {
		// Copy protocol to set our own debug logger
		protocol = reflect.New(reflect.ValueOf(protocol).Elem().Type()).Interface().(HubProtocol)
		protocol.SetDebugLogger(s.dbg)
		hubConn := newHubConnection(conn, protocol, s.info, s.dbg)
		// start sending pings to the client
		pings := startPingClientLoop(hubConn)
		hubConn.Start()
		s.lifetimeManager.OnConnected(hubConn)
		streamer := newStreamer(hubConn)
		streamClient := newStreamClient()
		s.getHub(hubConn).OnConnected(hubConn.GetConnectionID())
		// Process messages
		var message interface{}
		var connErr error
		messageLoop:
		for hubConn.IsConnected() {
			if message, connErr = hubConn.Receive(); connErr != nil {
				_ = info.Log(evt, msgRecv, "error", connErr, msg, message, react, "disconnect")
				break
			} else {
				switch message.(type) {
				case invocationMessage:
					invocation := message.(invocationMessage)
					_ = dbg.Log(evt, msgRecv, msg, invocation)
					// Transient hub
					// Dispatch invocation here
					if method, ok := getMethod(s.getHub(hubConn), invocation.Target); !ok {
						// Unable to find the method
						_ = info.Log(evt, "getMethod", "error", "missing method", "name", invocation.Target, react, "send completion with error")
						hubConn.Completion(invocation.InvocationID, nil, fmt.Sprintf("Unknown method %s", invocation.Target))
					} else if in, clientStreaming, err := buildMethodArguments(method, invocation, streamClient, protocol); err != nil {
						// argument build failed
						_ = info.Log(evt, "buildMethodArguments", "error", err, "name", invocation.Target, react, "send completion with error")
						hubConn.Completion(invocation.InvocationID, nil, err.Error())
					} else if clientStreaming {
						// let the receiving method run independently
						go func() {
							defer func() {
								if err := recover(); err != nil {
									_ = info.Log(evt, "recover", "error", err, "name", invocation.Target, react, "send completion with error")
									hubConn.Completion(invocation.InvocationID, nil, fmt.Sprintf("%v\n%v", err, string(debug.Stack())))
								}
							}()
							method.Call(in)
						}()
					} else {
						// hub method might take a long time
						go func() {
							result := func() []reflect.Value {
								defer func() {
									if err := recover(); err != nil {
										_ = info.Log(evt, "recover", "error", err, "name", invocation.Target, react, "send completion with error")
										hubConn.Completion(invocation.InvocationID, nil, fmt.Sprintf("%v\n%v", err, string(debug.Stack())))
									}
								}()
								return method.Call(in)
							}()
							returnInvocationResult(hubConn, invocation, streamer, result)
						}()
					}
				case cancelInvocationMessage:
					_ = dbg.Log(evt, msgRecv, msg, message.(cancelInvocationMessage))
					streamer.Stop(message.(cancelInvocationMessage).InvocationID)
				case streamItemMessage:
					streamItemMessage := message.(streamItemMessage)
					_ = dbg.Log(evt, msgRecv, msg, streamItemMessage)
					if err := streamClient.receiveStreamItem(streamItemMessage); err != nil {
						hubConn.Completion(streamItemMessage.InvocationID, nil,
							fmt.Sprintf(`stream item "%v" could not be received: %v`, streamItemMessage.Item, err))
					}
				case completionMessage:
					_ = dbg.Log(evt, msgRecv, msg, message.(completionMessage))
					streamClient.receiveCompletionItem(message.(completionMessage))
				case closeMessage:
					_ = dbg.Log(evt, msgRecv, msg, message.(closeMessage))
					break messageLoop
				case hubMessage:
					_ = dbg.Log(evt, msgRecv, msg, message)
					// Ping
				default:
					_ = info.Log(evt, msgRecv, "error", err, msg, message, react, "ignore")
				}
			}
		}
		s.getHub(hubConn).OnDisconnected(hubConn.GetConnectionID())
		s.lifetimeManager.OnDisconnected(hubConn)
		if connErr != nil {
			hubConn.Close(connErr.Error())
		} else {
			hubConn.Close("")
		}
		// Wait for pings to complete
		pings.Wait()
		_ = dbg.Log(evt, "messageloop ended")
	}
}

func (s *Server) prefixLogger() (info log.Logger, debug log.Logger) {
	return log.WithPrefix(s.info, "ts", log.DefaultTimestampUTC,
			"class", "Server",
			"hub", reflect.ValueOf(s.newHub()).Elem().Type()),
		log.WithPrefix(s.dbg, "ts", log.DefaultTimestampUTC,
			"class", "Server",
			"hub", reflect.ValueOf(s.newHub()).Elem().Type())
}

func buildInfoDebugLogger(logger log.Logger, debug bool) (log.Logger, log.Logger) {
	if debug {
		logger = level.NewFilter(logger, level.AllowDebug())
	} else {
		logger = level.NewFilter(logger, level.AllowInfo())
	}
	return level.Info(logger), log.With(level.Debug(logger), "caller", log.DefaultCaller)
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

func (s *Server) newConnectionHubContext(conn hubConnection) HubContext {
	return &connectionHubContext{
		clients: &callerHubClients{
			defaultHubClients: s.defaultHubClients,
			connectionID:      conn.GetConnectionID(),
		},
		groups: s.groupManager,
		items:  conn.Items(),
	}
}

func (s *Server) getHub(conn hubConnection) HubInterface {
	hub := s.newHub()
	hub.Initialize(s.newConnectionHubContext(conn))
	return hub
}

func getMethod(hub HubInterface, name string) (reflect.Value, bool) {
	hubType := reflect.TypeOf(hub)
	hubValue := reflect.ValueOf(hub)
	name = strings.ToLower(name)
	for i := 0; i < hubType.NumMethod(); i++ {
		if m := hubType.Method(i); strings.ToLower(m.Name) == name {
			return hubValue.Method(i), true
		}
	}
	return reflect.Value{}, false
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
	if len(invocation.StreamIds) > chanCount {
		return arguments, chanCount > 0, errors.New(fmt.Sprintf("to many StreamIds for channel parameters of method %v", invocation.Target))
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

func (s *Server) processHandshake(conn Connection) (HubProtocol, error) {
	var err error
	var protocol HubProtocol
	var ok bool
	const handshakeResponse = "{}\u001e"
	const errorHandshakeResponse = "{\"error\":\"%s\"}\u001e"
	info, dbg := s.prefixLogger()

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

		_ = dbg.Log(evt, "handshake received")

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
			_ = info.Log(evt, "protocol requested", "error", fmt.Sprintf("client requested unsupported protocol \"%s\" ", request.Protocol))
			_, err = conn.Write([]byte(fmt.Sprintf(errorHandshakeResponse, fmt.Sprintf("Protocol \"%s\" not supported", request.Protocol))))
		}
		break
	}

	// TODO Disable the timeout (either we already timeout out or)
	//ws.SetReadDeadline(time.Time{})

	return protocol, err
}

var protocolMap = map[string]HubProtocol{
	"json": &JSONHubProtocol{},
}

// const for logging
const evt string = "event"
const msgRecv string = "message received"
const msg string = "message"
const react string = "reaction"
