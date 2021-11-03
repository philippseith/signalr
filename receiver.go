package signalr

// ReceiverInterface allows receivers to interact with the server directly from the receiver methods
//  Init(Client)
// Init is used by the Client to connect the receiver to the server.
//  Server() Client
// Server can be used inside receiver methods to call Client methods,
// e.g. Client.Send, Client.Invoke, Client.PullStream and Client.PushStreams
type ReceiverInterface interface {
	Init(Client)
	Server() Client
}

// Receiver is a base class for receivers in the client.
// It implements ReceiverInterface
type Receiver struct {
	client Client
}

// Init is used by the Client to connect the receiver to the server.
func (ch *Receiver) Init(client Client) {
	ch.client = client
}

// Server can be used inside receiver methods to call Client methods,
func (ch *Receiver) Server() Client {
	return ch.client
}
