package signalr

import (
	"fmt"
	"github.com/go-kit/kit/log"
	"reflect"
	"runtime/debug"
	"sync"
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
	hubConn := newHubConnection(conn, protocol, s.info, s.dbg)
	return &serverLoop{
		server:       s,
		info:         info,
		dbg:          dbg,
		protocol:     protocol,
		hubConn:      hubConn,
		pings:        startPingClientLoop(hubConn),
		streamer:     newStreamer(hubConn),
		streamClient: newStreamClient(s.hubChanReceiveTimeout),
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
			switch message.(type) {
			case invocationMessage:
				sl.handleInvocationMessage(message)
			case cancelInvocationMessage:
				_ = sl.dbg.Log(evt, msgRecv, msg, message.(cancelInvocationMessage))
				sl.streamer.Stop(message.(cancelInvocationMessage).InvocationID)
			case streamItemMessage:
				connErr = sl.handleStreamItemMessage(message)
			case completionMessage:
				connErr = sl.handleCompletionMessage(message)
			case closeMessage:
				_ = sl.dbg.Log(evt, msgRecv, msg, message.(closeMessage))
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

func (sl *serverLoop) handleInvocationMessage(message interface{}) {
	invocation := message.(invocationMessage)
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

func (sl *serverLoop) handleStreamItemMessage(message interface{}) error {
	streamItemMessage := message.(streamItemMessage)
	_ = sl.dbg.Log(evt, msgRecv, msg, streamItemMessage)
	if err := sl.streamClient.receiveStreamItem(streamItemMessage); err != nil {
		switch t := err.(type) {
		case *hubChanTimeoutError:
			sl.hubConn.Completion(streamItemMessage.InvocationID, nil, t.Error())
		default:
			_ = sl.info.Log(evt, msgRecv, "error", err, msg, message, react, "disconnect")
			return err
		}
	}
	return nil
}

func (sl *serverLoop) handleCompletionMessage(message interface{}) error {
	_ = sl.dbg.Log(evt, msgRecv, msg, message.(completionMessage))
	var err error
	if err = sl.streamClient.receiveCompletionItem(message.(completionMessage)); err != nil {
		_ = sl.info.Log(evt, msgRecv, "error", err, msg, message, react, "disconnect")
	}
	return err
}

func (sl *serverLoop) handleOtherMessage(message interface{}) error {
	hubMessage := message.(hubMessage)
	_ = sl.dbg.Log(evt, msgRecv, msg, hubMessage)
	// Not Ping
	if hubMessage.Type != 6 {
		err := fmt.Errorf("invalid message type %v", message)
		_ = sl.info.Log(evt, msgRecv, "error", err, msg, message, react, "disconnect")
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
