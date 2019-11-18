package signalr

type Chat interface {
	Send(message string)
	Initialize(clients HubClients)
}

type chat struct {
	clients HubClients
}

func NewChat() Chat {
	return &chat{}
}

func (c *chat) Initialize(clients HubClients) {
	c.clients = clients
}

func (c *chat) Send(message string) {
	c.clients.All.Send("send", message)
}
