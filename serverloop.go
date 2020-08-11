package signalr

import (
	"context"
	"fmt"
	"reflect"
	"runtime/debug"
	"strings"
	"time"
)

type serverLoop struct {
	server         *Server
	info           StructuredLogger
	dbg            StructuredLogger
	protocol       HubProtocol
	hubConn        hubConnection
	allowReconnect bool
	streamer       *streamer
	streamClient   *streamClient
}

func (s *Server) newServerLoop(parentContext context.Context, conn Connection, protocol HubProtocol) *serverLoop {
	protocol = reflect.New(reflect.ValueOf(protocol).Elem().Type()).Interface().(HubProtocol)
	protocol.setDebugLogger(s.dbg)
	info, dbg := s.prefixLogger()
	hubConn := newHubConnection(parentContext, conn, protocol, s.maximumReceiveMessageSize)
	return &serverLoop{
		server:         s,
		protocol:       protocol,
		hubConn:        hubConn,
		allowReconnect: true,
		streamer:       newStreamer(hubConn, s.info),
		streamClient:   s.newStreamClient(),
		info:           info,
		dbg:            dbg,
	}
}

type loopEvent struct {
	message          interface{}
	err              error
	keepAliveTimeout bool
	clientTimeout    bool
}

func (sl *serverLoop) Run() {
	sl.hubConn.Start()
	sl.server.lifetimeManager.OnConnected(sl.hubConn)
	go func() {
		defer sl.recoverHubLifeCyclePanic()
		sl.server.getHub(sl.hubConn).OnConnected(sl.hubConn.ConnectionID())
	}()
	abortConnCh := sl.hubConn.Aborted()
	// Process messages
	var err error
loop:
	for {
		ch := make(chan loopEvent, 1)
		go func() {
			<-time.After(sl.server.keepAliveInterval)
			ch <- loopEvent{
				keepAliveTimeout: true,
			}
		}()
		go func() {
			<-time.After(sl.server.clientTimeoutInterval)
			ch <- loopEvent{
				clientTimeout: true,
			}
		}()
		go func() {
			message, err := sl.receive()
			ch <- loopEvent{
				message: message,
				err:     err,
			}
		}()
		select {
		case evt := <-ch:
			// First sender wins
			go func() {
				// Ignore the loosers
				<-ch
				<-ch
				close(ch)
			}()
			// Parse the winning event
			if evt.message != nil || evt.err != nil {
				err = evt.err
				if err == nil {
					switch message := evt.message.(type) {
					case invocationMessage:
						sl.handleInvocationMessage(message)
					case cancelInvocationMessage:
						_ = sl.dbg.Log(evt, msgRecv, msg, fmtMsg(message))
						sl.streamer.Stop(message.InvocationID)
					case streamItemMessage:
						err = sl.handleStreamItemMessage(message)
					case completionMessage:
						err = sl.handleCompletionMessage(message)
					case closeMessage:
						_ = sl.dbg.Log(evt, msgRecv, msg, fmtMsg(message))
						break loop
					case hubMessage:
						err = sl.handleOtherMessage(message)
						// No default case necessary, because the protocol would return either a hubMessage or an error
					}
				}
				if err != nil {
					break loop
				}
			} else if evt.keepAliveTimeout {
				sendMessageAndLog(func() (interface{}, error) { return sl.hubConn.Ping() }, sl.info)
			} else if evt.clientTimeout {
				err = fmt.Errorf("client timeout interval elapsed (%v)", sl.server.clientTimeoutInterval)
				break loop
			}
		case <-abortConnCh:
			break loop
		}
	}
	go func() {
		defer sl.recoverHubLifeCyclePanic()
		sl.server.getHub(sl.hubConn).OnDisconnected(sl.hubConn.ConnectionID())
	}()
	sl.server.lifetimeManager.OnDisconnected(sl.hubConn)
	sendMessageAndLog(func() (interface{}, error) {
		return sl.hubConn.Close(fmt.Sprintf("%v", err), sl.allowReconnect)
	}, sl.info)
	_ = sl.dbg.Log(evt, "message loop ended")
}

func (sl *serverLoop) receive() (message interface{}, err error) {
	if message, err = sl.hubConn.Receive(); err != nil {
		_ = sl.info.Log(evt, msgRecv, "error", err, msg, fmtMsg(message), react, "close connection")
	}
	return message, err
}

func (sl *serverLoop) handleInvocationMessage(invocation invocationMessage) {
	_ = sl.dbg.Log(evt, msgRecv, msg, fmtMsg(invocation))
	// Transient hub, dispatch invocation here
	if method, ok := getMethod(sl.server.getHub(sl.hubConn), invocation.Target); !ok {
		// Unable to find the method
		_ = sl.info.Log(evt, "getMethod", "error", "missing method", "name", invocation.Target, react, "send completion with error")
		sendMessageAndLog(func() (interface{}, error) {
			return sl.hubConn.Completion(invocation.InvocationID, nil, fmt.Sprintf("Unknown method %s", invocation.Target))
		}, sl.info)
	} else if in, clientStreaming, err := buildMethodArguments(method, invocation, sl.streamClient, sl.protocol); err != nil {
		// argument build failed
		_ = sl.info.Log(evt, "buildMethodArguments", "error", err, "name", invocation.Target, react, "send completion with error")
		sendMessageAndLog(func() (interface{}, error) {
			return sl.hubConn.Completion(invocation.InvocationID, nil, err.Error())
		}, sl.info)
	} else if clientStreaming {
		// let the receiving method run independently
		go func() {
			defer sl.recoverInvocationPanic(invocation)
			method.Call(in)
		}()
	} else {
		// hub method might take a long time
		go func() {
			result := func() []reflect.Value {
				defer sl.recoverInvocationPanic(invocation)
				return method.Call(in)
			}()
			sl.returnInvocationResult(invocation, result)
		}()
	}
}

func (sl *serverLoop) returnInvocationResult(invocation invocationMessage, result []reflect.Value) {
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
						sl.invokeConnection(invocation, completion, []reflect.Value{chanResult})
					} else {
						sendMessageAndLog(func() (interface{}, error) {
							return sl.hubConn.Completion(invocation.InvocationID, nil, "hub func returned closed chan")
						}, sl.info)
					}
				}()
			// StreamInvocation
			case 4:
				sl.streamer.Start(invocation.InvocationID, result[0])
			}
		} else {
			switch invocation.Type {
			// Simple invocation
			case 1:
				sl.invokeConnection(invocation, completion, result)
			case 4:
				// Stream invocation of method with no stream result.
				// Return a single StreamItem and an empty Completion
				sl.invokeConnection(invocation, streamItem, result)
				sendMessageAndLog(func() (interface{}, error) {
					return sl.hubConn.Completion(invocation.InvocationID, nil, "")
				}, sl.info)
			}
		}
	}
}

func (sl *serverLoop) handleStreamItemMessage(streamItemMessage streamItemMessage) error {
	_ = sl.dbg.Log(evt, msgRecv, msg, fmtMsg(streamItemMessage))
	if err := sl.streamClient.receiveStreamItem(streamItemMessage); err != nil {
		switch t := err.(type) {
		case *hubChanTimeoutError:
			sendMessageAndLog(func() (interface{}, error) {
				return sl.hubConn.Completion(streamItemMessage.InvocationID, nil, t.Error())
			}, sl.info)
		default:
			_ = sl.info.Log(evt, msgRecv, "error", err, msg, fmtMsg(streamItemMessage), react, "close connection")
			return err
		}
	}
	return nil
}

func (sl *serverLoop) handleCompletionMessage(message completionMessage) error {
	_ = sl.dbg.Log(evt, msgRecv, msg, fmtMsg(message))
	var err error
	if err = sl.streamClient.receiveCompletionItem(message); err != nil {
		_ = sl.info.Log(evt, msgRecv, "error", err, msg, fmtMsg(message), react, "close connection")
	}
	return err
}

func (sl *serverLoop) handleOtherMessage(hubMessage hubMessage) error {
	_ = sl.dbg.Log(evt, msgRecv, msg, fmtMsg(hubMessage))
	// Not Ping
	if hubMessage.Type != 6 {
		err := fmt.Errorf("invalid message type %v", hubMessage)
		_ = sl.info.Log(evt, msgRecv, "error", err, msg, fmtMsg(hubMessage), react, "close connection")
		return err
	}
	return nil
}

func (sl *serverLoop) recoverInvocationPanic(invocation invocationMessage) {
	if err := recover(); err != nil {
		_ = sl.info.Log(evt, "panic in hub method", "error", err, "name", invocation.Target, react, "send completion with error")
		stack := string(debug.Stack())
		_ = sl.dbg.Log(evt, "panic in hub method", "error", err, "name", invocation.Target, react, "send completion with error", "stack", stack)
		if invocation.InvocationID != "" {
			if !sl.server.enableDetailedErrors {
				stack = ""
			}
			sendMessageAndLog(func() (interface{}, error) {
				return sl.hubConn.Completion(invocation.InvocationID, nil, fmt.Sprintf("%v\n%v", err, stack))
			}, sl.info)
		}
	}
}

func (sl *serverLoop) recoverHubLifeCyclePanic() {
	if err := recover(); err != nil {
		sl.allowReconnect = false
		_ = sl.info.Log(evt, "panic in hub lifecycle", "error", err, react, "close connection, allow no reconnect")
		_ = sl.dbg.Log(evt, "panic in hub lifecycle", "error", err, react, "close connection, allow no reconnect", "stack", string(debug.Stack()))
		sl.hubConn.Abort()
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

func (sl *serverLoop) invokeConnection(invocation invocationMessage, connFunc connFunc, result []reflect.Value) {
	values := make([]interface{}, len(result))
	for i, rv := range result {
		values[i] = rv.Interface()
	}
	switch len(result) {
	case 0:
		sendMessageAndLog(func() (interface{}, error) {
			return sl.hubConn.Completion(invocation.InvocationID, nil, "")
		}, sl.info)
	case 1:
		connFunc(sl, invocation, values[0])
	default:
		connFunc(sl, invocation, values)
	}
}

type connFunc func(sl *serverLoop, invocation invocationMessage, value interface{})

func completion(sl *serverLoop, invocation invocationMessage, value interface{}) {
	sendMessageAndLog(func() (interface{}, error) {
		return sl.hubConn.Completion(invocation.InvocationID, value, "")
	}, sl.info)
}

func streamItem(sl *serverLoop, invocation invocationMessage, value interface{}) {
	sendMessageAndLog(func() (interface{}, error) {
		return sl.hubConn.StreamItem(invocation.InvocationID, value)
	}, sl.info)
}

func sendMessageAndLog(connFunc func() (interface{}, error), info StructuredLogger) {
	if msg, err := connFunc(); err != nil {
		_ = info.Log(evt, msgSend, "message", fmtMsg(msg), "error", err)
	}
}

func fmtMsg(msg interface{}) string {
	return fmt.Sprintf("%v", msg)
}
