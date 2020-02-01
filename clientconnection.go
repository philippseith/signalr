package signalr

import (
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
	panic("implement me")
}

type clientConnection struct {
	partyBase
	receiver interface{}
}

func (c *clientConnection) Start() <-chan error {
	panic("implement me")
}

func (c *clientConnection) Close() <-chan error {
	panic("implement me")
}

func (c *clientConnection) Closed() <-chan error {
	panic("implement me")
}

func (c *clientConnection) Invoke(method string, arguments ...interface{}) (<-chan interface{}, <-chan error) {
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

func (c *clientConnection) getInvocationTarget(hubConnection) interface{} {
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
