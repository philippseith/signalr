package signalr

import (
	"errors"
	"fmt"
	"reflect"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"time"
)

type loop struct {
	party        Party
	info         StructuredLogger
	dbg          StructuredLogger
	protocol     hubProtocol
	hubConn      hubConnection
	invokeClient *invokeClient
	streamer     *streamer
	streamClient *streamClient
	lastID       uint64
}

func newLoop(p Party, conn Connection, protocol hubProtocol) *loop {
	protocol = reflect.New(reflect.ValueOf(protocol).Elem().Type()).Interface().(hubProtocol)
	_, dbg := p.loggers()
	protocol.setDebugLogger(dbg)
	pInfo, pDbg := p.prefixLoggers(conn.ConnectionID())
	conn.SetTimeout(p.timeout())
	hubConn := newHubConnection(conn, protocol, p.maximumReceiveMessageSize(), pInfo)
	return &loop{
		party:        p,
		protocol:     protocol,
		hubConn:      hubConn,
		invokeClient: newInvokeClient(p.chanReceiveTimeout()),
		streamer:     &streamer{conn: hubConn},
		streamClient: newStreamClient(p.chanReceiveTimeout(), p.streamBufferCapacity()),
		info:         pInfo,
		dbg:          pDbg,
	}
}

var closeMessageError = errors.New("CloseMessage received")

// Run runs the loop. After the startup sequence is done, this is signaled over the started channel.
// Callers should pass a channel with buffer size 1 to allow the loop to run without waiting for the caller.
func (l *loop) Run(started chan struct{}) {
	l.party.onConnected(l.hubConn)
	started <- struct{}{}
	close(started)
	// Process messages
	var err error
	ch := make(chan receiveResult, 1)
	go func() {
		for result := range l.hubConn.Receive() {
			ch <- result
			// The loop ends when the chan returned by hubConn.Receive() is closed.
			// This happens when hubConn.Abort is called at the end to Run()
		}
	}()
msgLoop:
	for {
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
						err = closeMessageError
					case hubMessage:
						// Mostly ping
						err = l.handleOtherMessage(message)
						// No default case necessary, because the protocol would return either a hubMessage or an error
					}
				} else {
					_ = l.info.Log(evt, msgRecv, "error", err, msg, fmtMsg(evt.message), react, "close connection")
				}
				break pingLoop
			case <-time.After(l.party.keepAliveInterval()):
				// Send ping only when there was no write in the keepAliveInterval before
				if time.Since(l.hubConn.LastWriteStamp()) > l.party.keepAliveInterval() {
					_ = l.hubConn.Ping()
				}
				// Don't break the pingLoop, it exists for this case
			case <-time.After(l.party.timeout()):
				err = fmt.Errorf("client timeout interval elapsed (%v)", l.party.timeout())
				break pingLoop
			case <-l.hubConn.Context().Done():
				err = fmt.Errorf("breaking loop. hubConnection canceled: %w", l.hubConn.Context().Err())
				break pingLoop
			case <-l.party.context().Done():
				err = fmt.Errorf("breaking loop. Party canceled: %w", l.party.context().Err())
				break pingLoop
			}
		}
		if err != nil {
			break msgLoop
		}
	}
	l.party.onDisconnected(l.hubConn)
	// Don't send CloseMessage if we received a CloseMessage
	if err != closeMessageError {
		_ = l.hubConn.Close(fmt.Sprintf("%v", err), l.party.allowReconnect())
	}
	_ = l.dbg.Log(evt, "message loop ended")
	l.invokeClient.cancelAllInvokes()
	l.hubConn.Abort()
}

func (l *loop) PullStream(method, id string, arguments ...interface{}) <-chan InvokeResult {
	_, errChan := l.invokeClient.newInvocation(id)
	upChan := l.streamClient.newUpstreamChannel(id)
	ch := newInvokeResultChan(upChan, errChan)
	if err := l.hubConn.SendStreamInvocation(id, method, arguments, nil); err != nil {
		// When we get an error here, the loop is closed and the errChan might be already closed
		// We create a new one to deliver our error
		ch, _ = createResultChansWithError(err)
		l.streamClient.deleteUpstreamChannel(id)
		l.invokeClient.deleteInvocation(id)
	}
	return ch
}

func (l *loop) PushStreams(method, id string, arguments ...interface{}) <-chan error {
	_, errChan := l.invokeClient.newInvocation(id)
	invokeArgs := make([]interface{}, 0)
	reflectedChannels := make([]reflect.Value, 0)
	streamIds := make([]string, 0)
	// Parse arguments for channels and other kind of arguments
	for _, arg := range arguments {
		if reflect.TypeOf(arg).Kind() == reflect.Chan {
			reflectedChannels = append(reflectedChannels, reflect.ValueOf(arg))
			streamIds = append(streamIds, l.GetNewID())
		} else {
			invokeArgs = append(invokeArgs, arg)
		}
	}
	// Tell the server we are streaming now
	if err := l.hubConn.SendStreamInvocation(l.GetNewID(), method, invokeArgs, streamIds); err != nil {
		// When we get an error here, the loop is closed and the errChan might be already closed
		// We create a new one to deliver our error
		_, errChan = createResultChansWithError(err)
		l.invokeClient.deleteInvocation(id)
		return errChan
	}
	// Start streaming on all channels
	for i, reflectedChannel := range reflectedChannels {
		l.streamer.Start(streamIds[i], reflectedChannel)
	}
	return errChan
}

// GetNewID returns a new, connection-unique id for invocations and streams
func (l *loop) GetNewID() string {
	atomic.AddUint64(&l.lastID, 1)
	return fmt.Sprint(l.lastID)
}

func (l *loop) handleInvocationMessage(invocation invocationMessage) {
	_ = l.dbg.Log(evt, msgRecv, msg, fmtMsg(invocation))
	// Transient hub, dispatch invocation here
	if method, ok := getMethod(l.party.invocationTarget(l.hubConn), invocation.Target); !ok {
		// Unable to find the method
		_ = l.info.Log(evt, "getMethod", "error", "missing method", "name", invocation.Target, react, "send completion with error")
		_ = l.hubConn.Completion(invocation.InvocationID, nil, fmt.Sprintf("Unknown method %s", invocation.Target))
	} else if in, clientStreaming, err := buildMethodArguments(method, invocation, l.streamClient, l.protocol); err != nil {
		// argument build failed
		_ = l.info.Log(evt, "buildMethodArguments", "error", err, "name", invocation.Target, react, "send completion with error")
		_ = l.hubConn.Completion(invocation.InvocationID, nil, err.Error())
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
			_ = l.hubConn.Completion(invocation.InvocationID, nil,
				fmt.Sprintf("Stream invocation of method %s which has not return value kind channel", invocation.Target))
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

						_ = l.hubConn.Completion(invocation.InvocationID, nil, "hub func returned closed chan")
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
				_ = l.hubConn.Completion(invocation.InvocationID, nil, "")
			}
		}
	}
}

func (l *loop) handleStreamItemMessage(streamItemMessage streamItemMessage) error {
	_ = l.dbg.Log(evt, msgRecv, msg, fmtMsg(streamItemMessage))
	if err := l.streamClient.receiveStreamItem(streamItemMessage); err != nil {
		switch t := err.(type) {
		case *hubChanTimeoutError:
			_ = l.hubConn.Completion(streamItemMessage.InvocationID, nil, t.Error())
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
		_ = l.hubConn.Completion(invocation.InvocationID, nil, "")
	case 1:
		connFunc(l, invocation, values[0])
	default:
		connFunc(l, invocation, values)
	}
}

type connFunc func(sl *loop, invocation invocationMessage, value interface{})

func completion(sl *loop, invocation invocationMessage, value interface{}) {
	_ = sl.hubConn.Completion(invocation.InvocationID, value, "")
}

func streamItem(sl *loop, invocation invocationMessage, value interface{}) {

	_ = sl.hubConn.StreamItem(invocation.InvocationID, value)
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
			_ = l.hubConn.Completion(invocation.InvocationID, nil, fmt.Sprintf("%v\n%v", err, stack))
		}
	}
}

func buildMethodArguments(method reflect.Value, invocation invocationMessage,
	streamClient *streamClient, protocol hubProtocol) (arguments []reflect.Value, clientStreaming bool, err error) {
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

func fmtMsg(message interface{}) string {
	return fmt.Sprintf("%#v", message)
}
