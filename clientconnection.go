package signalr

import (
	"context"
	"github.com/go-kit/kit/log"
	"reflect"
)

type ClientConnection interface {
	party
	Start() <-chan error
	Close() <-chan error
	Closed() <-chan error
	Invoke(method string, arguments ...interface{}) (<-chan interface{}, <-chan error)
	Stream(method string, arguments ...interface{}) (<-chan interface{}, <-chan error)
	Upstream(method string, arguments ...interface{}) <-chan error
	// It is not necessary to register callbacks with On(...),
	// the server can "call back" all exported methods of the receiver
	SetReceiver(receiver interface{})
}

func NewClientConnection(conn Connection, options ...func(party) error) (ClientConnection, error) {
	c := &clientConnection{conn: conn}
	for _, option := range options {
		if option != nil {
			if err := option(c); err != nil {
				return nil, err
			}
		}
	}
	return c, nil
}

type clientConnection struct {
	partyBase
	conn     Connection
	cancel   context.CancelFunc
	loop     *loop
	receiver interface{}
}

func (c *clientConnection) Start() <-chan error {
	errCh := make(chan error, 1)
	if protocol, err := c.processHandshake(c.conn); err != nil {
		errCh <- err
	} else {
		var ctx context.Context
		ctx, c.cancel = context.WithCancel(context.Background())
		c.loop = newLoop(c, ctx, c.conn, protocol)
		c.loop.Run()
	}
	return errCh
}

func (c *clientConnection) Close() <-chan error {
	c.loop.hubConn.Close("", false)
	panic("implement me")
}

func (c *clientConnection) Closed() <-chan error {
	panic("implement me")
}

func (c *clientConnection) Invoke(method string, arguments ...interface{}) (<-chan interface{}, <-chan error) {
	c.loop.hubConn.SendInvocation(getID(), method, arguments)
	panic("implement me")
}

func (c *clientConnection) Stream(method string, arguments ...interface{}) (<-chan interface{}, <-chan error) {
	panic("implement me")
}

func (c *clientConnection) Upstream(method string, arguments ...interface{}) <-chan error {
	panic("implement me")
}

func (c *clientConnection) SetReceiver(receiver interface{}) {
	c.receiver = receiver
}

func (c *clientConnection) onConnected(hubConnection) {}

func (c *clientConnection) onDisconnected(hubConnection) {}

func (c *clientConnection) invocationTarget(hubConnection) interface{} {
	return c.receiver
}

func (c *clientConnection) allowReconnect() bool {
	return false // Servers don't care?
}

func (c *clientConnection) prefixLoggers() (info StructuredLogger, dbg StructuredLogger) {
	return log.WithPrefix(c.info, "ts", log.DefaultTimestampUTC,
			"class", "Client",
			"hub", reflect.ValueOf(c.receiver).Elem().Type()), log.WithPrefix(c.dbg, "ts", log.DefaultTimestampUTC,
			"class", "Client",
			"hub", reflect.ValueOf(c.receiver).Elem().Type())
}

func (c *clientConnection) processHandshake(conn Connection) (HubProtocol, error) {
	panic("implement me")
}
