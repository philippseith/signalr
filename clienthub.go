package signalr

// ClientHubInterface allows receivers to interact with the server directly from the receiver methods
//  Init(Client)
// Init is used by the Client to connect the receiver to the server.
//  Server() Client
// Server can be used inside receiver methods to call Client methods,
// e.g. Client.Send, Client.Invoke, Client.PullStream and Client.PushStreams
type ClientHubInterface interface {
	Init(Client)
	Server() Client
}

// ClientHub is a base class for receivers in the client.
// It implements ClientHubInterface
type ClientHub struct {
	Hub
	client Client
}

func (ch *ClientHub) Init(client Client) {
	ch.client = client
}
func (ch *ClientHub) Server() Client {
	return ch.client
}
