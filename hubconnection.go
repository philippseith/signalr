package signalr

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"time"
)

// hubConnection is used by HubContext, Server and Client to realize the external API.
// hubConnection uses a transport connection (of type Connection) and a hubProtocol to send and receive SignalR messages.
type hubConnection interface {
	ConnectionID() string
	Receive() <-chan receiveResult
	SendInvocation(id string, target string, args []interface{}) error
	SendStreamInvocation(id string, target string, args []interface{}) error
	SendInvocationWithStreamIds(id string, target string, args []interface{}, streamIds []string) error
	StreamItem(id string, item interface{}) error
	Completion(id string, result interface{}, error string) error
	Close(error string, allowReconnect bool) error
	Ping() error
	LastWriteStamp() time.Time
	Items() *sync.Map
	Context() context.Context
	Abort()
}

type receiveResult struct {
	message interface{}
	err     error
}

func newHubConnection(connection Connection, protocol hubProtocol, maximumReceiveMessageSize uint, info StructuredLogger) hubConnection {
	ctx, cancelFunc := context.WithCancel(connection.Context())
	c := &defaultHubConnection{
		ctx:                       ctx,
		cancelFunc:                cancelFunc,
		protocol:                  protocol,
		mx:                        sync.Mutex{},
		connection:                connection,
		maximumReceiveMessageSize: maximumReceiveMessageSize,
		items:                     &sync.Map{},
		info:                      info,
	}
	if connectionWithTransferMode, ok := connection.(ConnectionWithTransferMode); ok {
		connectionWithTransferMode.SetTransferMode(protocol.transferMode())
	}
	return c
}

type defaultHubConnection struct {
	ctx                       context.Context
	cancelFunc                context.CancelFunc
	protocol                  hubProtocol
	mx                        sync.Mutex
	connection                Connection
	maximumReceiveMessageSize uint
	items                     *sync.Map
	lastWriteStamp            time.Time
	info                      StructuredLogger
}

func (c *defaultHubConnection) Items() *sync.Map {
	return c.items
}

func (c *defaultHubConnection) Close(errorText string, allowReconnect bool) error {
	var closeMessage = closeMessage{
		Type:           7,
		Error:          errorText,
		AllowReconnect: allowReconnect,
	}
	return c.protocol.WriteMessage(closeMessage, c.connection)
}

func (c *defaultHubConnection) ConnectionID() string {
	return c.connection.ConnectionID()
}

func (c *defaultHubConnection) Context() context.Context {
	return c.ctx
}

func (c *defaultHubConnection) Abort() {
	c.cancelFunc()
}

func (c *defaultHubConnection) Receive() <-chan receiveResult {
	recvChan := make(chan receiveResult, 20)
	// Prepare cleanup
	writerDone := make(chan struct{}, 1)
	// the pipe connects the goroutine which reads from the connection and the goroutine which parses the read data
	reader, writer := CtxPipe(c.ctx)
	p := make([]byte, c.maximumReceiveMessageSize)
	go func(ctx context.Context, connection io.Reader, writer io.Writer, recvChan chan<- receiveResult, writerDone chan<- struct{}) {
	loop:
		for {
			select {
			case <-ctx.Done():
				break loop
			default:
				n, err := connection.Read(p)
				if err != nil {
					select {
					case recvChan <- receiveResult{err: err}:
					case <-ctx.Done():
						break loop
					}
				}
				if n > 0 {
					_, err = writer.Write(p[:n])
					if err != nil {
						select {
						case recvChan <- receiveResult{err: err}:
						case <-ctx.Done():
							break loop
						}
					}
				}
			}
		}
		// The pipe writer is done
		close(writerDone)
	}(c.ctx, c.connection, writer, recvChan, writerDone)
	// parse
	go func(ctx context.Context, reader io.Reader, recvChan chan<- receiveResult, writerDone <-chan struct{}) {
		remainBuf := bytes.Buffer{}
	loop:
		for {
			select {
			case <-ctx.Done():
				break loop
			case <-writerDone:
				break loop
			default:
				messages, err := c.protocol.ParseMessages(reader, &remainBuf)
				if err != nil {
					select {
					case recvChan <- receiveResult{err: err}:
					case <-ctx.Done():
						break loop
					case <-writerDone:
						break loop
					}
				} else {
					for _, message := range messages {
						select {
						case recvChan <- receiveResult{message: message}:
						case <-ctx.Done():
							break loop
						case <-writerDone:
							break loop
						}
					}
				}
			}
		}
	}(c.ctx, reader, recvChan, writerDone)
	return recvChan
}

func (c *defaultHubConnection) SendInvocation(id string, target string, args []interface{}) error {
	if args == nil {
		args = make([]interface{}, 0)
	}
	var invocationMessage = invocationMessage{
		Type:         1,
		InvocationID: id,
		Target:       target,
		Arguments:    args,
	}
	return c.writeMessage(invocationMessage)
}

func (c *defaultHubConnection) SendStreamInvocation(id string, target string, args []interface{}) error {
	if args == nil {
		args = make([]interface{}, 0)
	}
	var invocationMessage = invocationMessage{
		Type:         4,
		InvocationID: id,
		Target:       target,
		Arguments:    args,
	}
	return c.writeMessage(invocationMessage)
}

func (c *defaultHubConnection) SendInvocationWithStreamIds(id string, target string, args []interface{}, streamIds []string) error {
	var invocationMessage = invocationMessage{
		Type:         1,
		InvocationID: id,
		Target:       target,
		Arguments:    args,
		StreamIds:    streamIds,
	}
	return c.writeMessage(invocationMessage)
}

func (c *defaultHubConnection) StreamItem(id string, item interface{}) error {
	var streamItemMessage = streamItemMessage{
		Type:         2,
		InvocationID: id,
		Item:         item,
	}
	return c.writeMessage(streamItemMessage)
}

func (c *defaultHubConnection) Completion(id string, result interface{}, error string) error {
	var completionMessage = completionMessage{
		Type:         3,
		InvocationID: id,
		Result:       result,
		Error:        error,
	}
	return c.writeMessage(completionMessage)
}

func (c *defaultHubConnection) Ping() error {
	var pingMessage = hubMessage{
		Type: 6,
	}
	return c.writeMessage(pingMessage)
}

func (c *defaultHubConnection) LastWriteStamp() time.Time {
	defer c.mx.Unlock()
	c.mx.Lock()
	return c.lastWriteStamp
}

func (c *defaultHubConnection) writeMessage(message interface{}) error {
	c.mx.Lock()
	c.lastWriteStamp = time.Now()
	c.mx.Unlock()
	err := func() error {
		if c.ctx.Err() != nil {
			return fmt.Errorf("hubConnection canceled: %w", c.ctx.Err())
		}
		e := make(chan error, 1)
		go func() { e <- c.protocol.WriteMessage(message, c.connection) }()
		select {
		case <-c.ctx.Done():
			return fmt.Errorf("hubConnection canceled: %w", c.ctx.Err())
		case err := <-e:
			if err != nil {
				c.Abort()
			}
			return err
		}
	}()
	if err != nil {
		_ = c.info.Log(evt, msgSend, "message", fmtMsg(message), "error", err)
	}
	return err
}
