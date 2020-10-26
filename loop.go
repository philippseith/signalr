package signalr

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"runtime/debug"
	"strings"
	"time"
)

type loop struct {
	party        party
	info         StructuredLogger
	dbg          StructuredLogger
	protocol     HubProtocol
	hubConn      hubConnection
	invokeClient *invokeClient
	streamer     *streamer
	streamClient *streamClient
}

func newLoop(parentContext context.Context, p party, conn Connection, protocol HubProtocol) *loop {
	protocol = reflect.New(reflect.ValueOf(protocol).Elem().Type()).Interface().(HubProtocol)
	info, dbg := p.loggers()
	protocol.setDebugLogger(dbg)
	pInfo, pDbg := p.prefixLoggers(conn.ConnectionID())
	hubConn := newHubConnection(parentContext, conn, protocol, p.maximumReceiveMessageSize())
	return &loop{
		party:        p,
		protocol:     protocol,
		hubConn:      hubConn,
		invokeClient: newInvokeClient(p.chanReceiveTimeout()),
		streamer:     newStreamer(hubConn, info),
		streamClient: newStreamClient(p.chanReceiveTimeout(), p.streamBufferCapacity()),
		info:         pInfo,
		dbg:          pDbg,
	}
}

type loopEvent struct {
	message          interface{}
	err              error
	keepAliveTimeout bool
	clientTimeout    bool
}

func (l *loop) Run() {
	l.hubConn.Start()
	l.party.onConnected(l.hubConn)
	abortConnCh := l.hubConn.Aborted()
	// Process messages
	var err error
msgLoop:
	for {
		ch := make(chan loopEvent, 1)
		go func() {
			message, err := l.receive()
			ch <- loopEvent{
				message: message,
				err:     err,
			}
			close(ch)
		}()
	pingLoop:
		for {
			select {
			case evt := <-ch:
				err = evt.err
				if err == nil {
					switch message := evt.message.(type) {
					case invocationMessage:
						l.handleInvocationMessage(message)
					case cancelInvocationMessage:
						_ = l.dbg.Log(evt, msgRecv, msg, fmtMsg(message))
						l.streamer.Stop(message.InvocationID)
					case streamItemMessage:
						err = l.handleStreamItemMessage(message)
					case completionMessage:
						err = l.handleCompletionMessage(message)
					case closeMessage:
						_ = l.dbg.Log(evt, msgRecv, msg, fmtMsg(message))
						// Bogus error to break the msgLoop
						err = errors.New("")
					case hubMessage:
						// Mostly ping
						err = l.handleOtherMessage(message)
						// No default case necessary, because the protocol would return either a hubMessage or an error
					}
				}
				break pingLoop
			case <-time.After(l.party.keepAliveInterval()):
				sendMessageAndLog(func() (interface{}, error) { return l.hubConn.Ping() }, l.info)
				// Don't break the pingLoop, it exists for this case
			case <-time.After(l.party.timeout()):
				err = fmt.Errorf("client timeout interval elapsed (%v)", l.party.timeout())
				break pingLoop
			case err = <-abortConnCh:
				break pingLoop
			}
		}
		if err != nil {
			break msgLoop
		}
	}
	l.party.onDisconnected(l.hubConn)
	sendMessageAndLog(func() (interface{}, error) {
		return l.hubConn.Close(fmt.Sprintf("%v", err), l.party.allowReconnect())
	}, l.info)
	_ = l.dbg.Log(evt, "message loop ended")
	l.invokeClient.cancelAllInvokes()
}

func (l *loop) receive() (message interface{}, err error) {
	if message, err = l.hubConn.Receive(); err != nil {
		_ = l.info.Log(evt, msgRecv, "error", err, msg, fmtMsg(message), react, "close connection")
	}
	return message, err
}

func (l *loop) handleInvocationMessage(invocation invocationMessage) {
	_ = l.dbg.Log(evt, msgRecv, msg, fmtMsg(invocation))
	// Transient hub, dispatch invocation here
	if method, ok := getMethod(l.party.invocationTarget(l.hubConn), invocation.Target); !ok {
		// Unable to find the method
		_ = l.info.Log(evt, "getMethod", "error", "missing method", "name", invocation.Target, react, "send completion with error")
		sendMessageAndLog(func() (interface{}, error) {
			return l.hubConn.Completion(invocation.InvocationID, nil, fmt.Sprintf("Unknown method %s", invocation.Target))
		}, l.info)
	} else if in, clientStreaming, err := buildMethodArguments(method, invocation, l.streamClient, l.protocol); err != nil {
		// argument build failed
		_ = l.info.Log(evt, "buildMethodArguments", "error", err, "name", invocation.Target, react, "send completion with error")
		sendMessageAndLog(func() (interface{}, error) {
			return l.hubConn.Completion(invocation.InvocationID, nil, err.Error())
		}, l.info)
	} else if clientStreaming {
		// let the receiving method run independently
		go func() {
			defer l.recoverInvocationPanic(invocation)
			method.Call(in)
		}()
	} else {
		// Stream invocation is only allowed when the method has only one return value
		// We allow no channel return values, because a client can receive as stream with only one item
		if invocation.Type == 4 && method.Type().NumOut() != 1 {
			sendMessageAndLog(func() (interface{}, error) {
				return l.hubConn.Completion(invocation.InvocationID, nil,
					fmt.Sprintf("Stream invocation of method %s which has not return value kind channel", invocation.Target))
			}, l.info)
		} else {
			// hub method might take a long time
			go func() {
				result := func() []reflect.Value {
					defer l.recoverInvocationPanic(invocation)
					return method.Call(in)
				}()
				l.returnInvocationResult(invocation, result)
			}()
		}
	}
}

func (l *loop) returnInvocationResult(invocation invocationMessage, result []reflect.Value) {
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
						l.sendResult(invocation, completion, []reflect.Value{chanResult})
					} else {
						sendMessageAndLog(func() (interface{}, error) {
							return l.hubConn.Completion(invocation.InvocationID, nil, "hub func returned closed chan")
						}, l.info)
					}
				}()
			// StreamInvocation
			case 4:
				l.streamer.Start(invocation.InvocationID, result[0])
			}
		} else {
			switch invocation.Type {
			// Simple invocation
			case 1:
				l.sendResult(invocation, completion, result)
			case 4:
				// Stream invocation of method with no stream result.
				// Return a single StreamItem and an empty Completion
				l.sendResult(invocation, streamItem, result)
				sendMessageAndLog(func() (interface{}, error) {
					return l.hubConn.Completion(invocation.InvocationID, nil, "")
				}, l.info)
			}
		}
	}
}

func (l *loop) handleStreamItemMessage(streamItemMessage streamItemMessage) error {
	_ = l.dbg.Log(evt, msgRecv, msg, fmtMsg(streamItemMessage))
	if err := l.streamClient.receiveStreamItem(streamItemMessage); err != nil {
		switch t := err.(type) {
		case *hubChanTimeoutError:
			sendMessageAndLog(func() (interface{}, error) {
				return l.hubConn.Completion(streamItemMessage.InvocationID, nil, t.Error())
			}, l.info)
		default:
			_ = l.info.Log(evt, msgRecv, "error", err, msg, fmtMsg(streamItemMessage), react, "close connection")
			return err
		}
	}
	return nil
}

func (l *loop) handleCompletionMessage(message completionMessage) error {
	_ = l.dbg.Log(evt, msgRecv, msg, fmtMsg(message))
	var err error
	if l.streamClient.handlesInvocationID(message.InvocationID) {
		err = l.streamClient.receiveCompletionItem(message, l.invokeClient)
	} else if l.invokeClient.handlesInvocationID(message.InvocationID) {
		err = l.invokeClient.receiveCompletionItem(message)
	} else {
		err = fmt.Errorf("unkown invocationID %v", message.InvocationID)
	}
	if err != nil {
		_ = l.info.Log(evt, msgRecv, "error", err, msg, fmtMsg(message), react, "close connection")
	}
	return err
}

func (l *loop) handleOtherMessage(hubMessage hubMessage) error {
	_ = l.dbg.Log(evt, msgRecv, msg, fmtMsg(hubMessage))
	// Not Ping
	if hubMessage.Type != 6 {
		err := fmt.Errorf("invalid message type %v", hubMessage)
		_ = l.info.Log(evt, msgRecv, "error", err, msg, fmtMsg(hubMessage), react, "close connection")
		return err
	}
	return nil
}

func (l *loop) sendResult(invocation invocationMessage, connFunc connFunc, result []reflect.Value) {
	values := make([]interface{}, len(result))
	for i, rv := range result {
		values[i] = rv.Interface()
	}
	switch len(result) {
	case 0:
		sendMessageAndLog(func() (interface{}, error) {
			return l.hubConn.Completion(invocation.InvocationID, nil, "")
		}, l.info)
	case 1:
		connFunc(l, invocation, values[0])
	default:
		connFunc(l, invocation, values)
	}
}

type connFunc func(sl *loop, invocation invocationMessage, value interface{})

func completion(sl *loop, invocation invocationMessage, value interface{}) {
	sendMessageAndLog(func() (interface{}, error) {
		return sl.hubConn.Completion(invocation.InvocationID, value, "")
	}, sl.info)
}

func streamItem(sl *loop, invocation invocationMessage, value interface{}) {
	sendMessageAndLog(func() (interface{}, error) {
		return sl.hubConn.StreamItem(invocation.InvocationID, value)
	}, sl.info)
}

func (l *loop) recoverInvocationPanic(invocation invocationMessage) {
	if err := recover(); err != nil {
		_ = l.info.Log(evt, "panic in target method", "error", err, "name", invocation.Target, react, "send completion with error")
		stack := string(debug.Stack())
		_ = l.dbg.Log(evt, "panic in target method", "error", err, "name", invocation.Target, react, "send completion with error", "stack", stack)
		if invocation.InvocationID != "" {
			if !l.party.enableDetailedErrors() {
				stack = ""
			}
			sendMessageAndLog(func() (interface{}, error) {
				return l.hubConn.Completion(invocation.InvocationID, nil, fmt.Sprintf("%v\n%v", err, stack))
			}, l.info)
		}
	}
}

func buildMethodArguments(method reflect.Value, invocation invocationMessage,
	streamClient *streamClient, protocol HubProtocol) (arguments []reflect.Value, clientStreaming bool, err error) {
	if len(invocation.StreamIds)+len(invocation.Arguments) != method.Type().NumIn() {
		return nil, false, fmt.Errorf("parameter mismatch calling method %v", invocation.Target)
	}
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
	if len(invocation.StreamIds) != chanCount {
		return arguments, chanCount > 0, fmt.Errorf("to many StreamIds for channel parameters of method %v", invocation.Target)
	}
	return arguments, chanCount > 0, nil
}

func getMethod(target interface{}, name string) (reflect.Value, bool) {
	hubType := reflect.TypeOf(target)
	hubValue := reflect.ValueOf(target)
	name = strings.ToLower(name)
	for i := 0; i < hubType.NumMethod(); i++ {
		// Search in public methods
		if m := hubType.Method(i); strings.ToLower(m.Name) == name {
			return hubValue.Method(i), true
		}
	}
	return reflect.Value{}, false
}

// TODO Move logging to defaultHubConnection
func sendMessageAndLog(connFunc func() (interface{}, error), info StructuredLogger) {
	if msg, err := connFunc(); err != nil {
		_ = info.Log(evt, msgSend, "message", fmtMsg(msg), "error", err)
	}
}

func fmtMsg(msg interface{}) string {
	return fmt.Sprintf("%#v", msg)
}
