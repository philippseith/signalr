package signalr

type Client interface {
	Invoke(method string, arguments ...interface{}) <-chan interface{}
	StreamInvoke(method string, arguments ...interface{}) <-chan interface{}
	// It is not necessary to register callbacks with On(...),
	// the server can call back all exported methods of the receiver
	SetReceiver(receiver interface{})
}
