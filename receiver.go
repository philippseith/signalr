package signalr

// ReceiverInterface allows receivers to interact with the server directly from the receiver methods
//
//	Init(Client)
//
// Init is used by the Client to connect the receiver to the server.
//
//	Server() Client
//
// Server can be used inside receiver methods to call Client methods,
// e.g. Client.Send, Client.Invoke, Client.PullStream and Client.PushStreams
//
//	GetFunc(string) interface{}
//
// GetFunc is used to get the handler function by handler name.
//
//	SetFunc(string, interface{})
//
// SetFunc is used to set the mapping between the name and the handler function.
type ReceiverInterface interface {
	Init(Client)
	Server() Client
	GetFunc(string) interface{}
	SetFunc(string, interface{})
}

// Receiver is a base class for receivers in the client.
// It implements ReceiverInterface
type Receiver struct {
	methodMap map[string]interface{}
	client    Client
}

// Init is used by the Client to connect the receiver to the server.
func (ch *Receiver) Init(client Client) {
	ch.client = client
}

// Server can be used inside receiver methods to call Client methods,
func (ch *Receiver) Server() Client {
	return ch.client
}

// GetFunc is used to get the handler function by handler name.
func (ch *Receiver) GetFunc(name string) interface{} {
	return ch.methodMap[name]
}

// SetFunc is used to set the mapping between the name and the handler function.
func (ch *Receiver) SetFunc(name string, fn interface{}) {
	if ch.methodMap == nil {
		ch.methodMap = make(map[string]interface{})
	}
	if _, ok := ch.methodMap[name]; ok {
		info, _ := ch.client.loggers()
		_ = info.Log(evt, "set function", "warn", "method has been overridden", "method", name)
	}
	ch.methodMap[name] = fn
}

// On is an alias for SetFunc
func (ch *Receiver) On(name string, fn interface{}) {
	ch.SetFunc(name, fn)
}
