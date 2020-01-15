package signalr

import (
	"fmt"
	"github.com/go-kit/kit/log"
	"reflect"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

type serverLoop struct {
	server       *Server
	info         StructuredLogger
	dbg          StructuredLogger
	protocol     HubProtocol
	hubConn      hubConnection
	pings        *sync.WaitGroup
	streamer     *streamer
	streamClient *streamClient
}

func (s *Server) newServerLoop(conn Connection, protocol HubProtocol) *serverLoop {
	info, dbg := s.prefixLogger()
	protocol = reflect.New(reflect.ValueOf(protocol).Elem().Type()).Interface().(HubProtocol)
	protocol.setDebugLogger(s.dbg)
	hubConn := newHubConnection(conn, protocol, s.maximumReceiveMessageSize, s.info, s.dbg)
	return &serverLoop{
		server:       s,
		info:         info,
		dbg:          dbg,
		protocol:     protocol,
		hubConn:      hubConn,
		pings:        startPingClientLoop(hubConn),
		streamer:     newStreamer(hubConn),
		streamClient: s.newStreamClient(),
	}
}

func (sl *serverLoop) Run() {
	sl.hubConn.Start()
	sl.server.lifetimeManager.OnConnected(sl.hubConn)
	sl.server.getHub(sl.hubConn).OnConnected(sl.hubConn.GetConnectionID())
	// Process messages
	var message interface{}
	var connErr error
messageLoop:
	for sl.hubConn.IsConnected() {
		if message, connErr = sl.hubConn.Receive(); connErr != nil {
			_ = sl.info.Log(evt, msgRecv, "error", connErr, msg, message, react, "disconnect")
			break messageLoop
		} else {
			switch message := message.(type) {
			case invocationMessage:
				sl.handleInvocationMessage(message)
			case cancelInvocationMessage:
				_ = sl.dbg.Log(evt, msgRecv, msg, message)
				sl.streamer.Stop(message.InvocationID)
			case streamItemMessage:
				connErr = sl.handleStreamItemMessage(message)
			case completionMessage:
				connErr = sl.handleCompletionMessage(message)
			case closeMessage:
				_ = sl.dbg.Log(evt, msgRecv, msg, message)
				break messageLoop
			case hubMessage:
				connErr = sl.handleOtherMessage(message)
			}
			if connErr != nil {
				break
			}
		}
	}
	sl.server.getHub(sl.hubConn).OnDisconnected(sl.hubConn.GetConnectionID())
	sl.server.lifetimeManager.OnDisconnected(sl.hubConn)
	sl.hubConn.Close(fmt.Sprintf("%v", connErr))
	// Wait for pings to complete
	sl.pings.Wait()
	_ = sl.dbg.Log(evt, "messageloop ended")
}

func (sl *serverLoop) handleInvocationMessage(invocation invocationMessage) {
	_ = sl.dbg.Log(evt, msgRecv, msg, fmt.Sprintf("%v", invocation))
	// Transient hub, dispatch invocation here
	if method, ok := getMethod(sl.server.getHub(sl.hubConn), invocation.Target); !ok {
		// Unable to find the method
		_ = sl.info.Log(evt, "getMethod", "error", "missing method", "name", invocation.Target, react, "send completion with error")
		sl.hubConn.Completion(invocation.InvocationID, nil, fmt.Sprintf("Unknown method %s", invocation.Target))
	} else if in, clientStreaming, err := buildMethodArguments(method, invocation, sl.streamClient, sl.protocol); err != nil {
		// argument build failed
		_ = sl.info.Log(evt, "buildMethodArguments", "error", err, "name", invocation.Target, react, "send completion with error")
		sl.hubConn.Completion(invocation.InvocationID, nil, err.Error())
	} else if clientStreaming {
		// let the receiving method run independently
		go func() {
			defer recoverInvocationPanic(sl.info, invocation, sl.hubConn)
			method.Call(in)
		}()
	} else {
		// hub method might take a long time
		go func() {
			result := func() []reflect.Value {
				defer recoverInvocationPanic(sl.info, invocation, sl.hubConn)
				return method.Call(in)
			}()
			returnInvocationResult(sl.hubConn, invocation, sl.streamer, result)
		}()
	}
}

func returnInvocationResult(conn hubConnection, invocation invocationMessage, streamer *streamer, result []reflect.Value) {
	// No invocation id, no completion
	if invocation.InvocationID != "" {
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
}

func (sl *serverLoop) handleStreamItemMessage(streamItemMessage streamItemMessage) error {
	_ = sl.dbg.Log(evt, msgRecv, msg, streamItemMessage)
	if err := sl.streamClient.receiveStreamItem(streamItemMessage); err != nil {
		switch t := err.(type) {
		case *hubChanTimeoutError:
			sl.hubConn.Completion(streamItemMessage.InvocationID, nil, t.Error())
		default:
			_ = sl.info.Log(evt, msgRecv, "error", err, msg, streamItemMessage, react, "disconnect")
			return err
		}
	}
	return nil
}

func (sl *serverLoop) handleCompletionMessage(message completionMessage) error {
	_ = sl.dbg.Log(evt, msgRecv, msg, message)
	var err error
	if err = sl.streamClient.receiveCompletionItem(message); err != nil {
		_ = sl.info.Log(evt, msgRecv, "error", err, msg, message, react, "disconnect")
	}
	return err
}

func (sl *serverLoop) handleOtherMessage(hubMessage hubMessage) error {
	_ = sl.dbg.Log(evt, msgRecv, msg, hubMessage)
	// Not Ping
	if hubMessage.Type != 6 {
		err := fmt.Errorf("invalid message type %v", hubMessage)
		_ = sl.info.Log(evt, msgRecv, "error", err, msg, hubMessage, react, "disconnect")
		return err
	}
	return nil
}

func recoverInvocationPanic(info log.Logger, invocation invocationMessage, hubConn hubConnection) {
	if err := recover(); err != nil {
		_ = info.Log(evt, "recover", "error", err, "name", invocation.Target, react, "send completion with error")
		if invocation.InvocationID != "" {
			hubConn.Completion(invocation.InvocationID, nil, fmt.Sprintf("%v\n%v", err, string(debug.Stack())))
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
		return arguments, chanCount > 0, fmt.Errorf("to many StreamIds for channel parameters of method %v", invocation.Target)
	}
	return arguments, chanCount > 0, nil
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
