package signalr

type ClientConnection interface {
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
