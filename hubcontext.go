package signalr

import "sync"

// HubContext is a context abstraction for a hub
// Clients() gets a HubClients that can be used to invoke methods on clients connected to the hub
// Groups() gets a GroupManager that can be used to add and remove connections to named groups
// Items() holds key/value pairs scoped to the hubs connection
// ConnectionID() gets the ID of the current connection
// Abort() aborts the current connection
type HubContext interface {
	Clients() HubClients
	Groups() GroupManager
	Items() *sync.Map
	ConnectionID() string
	Abort()
}

type connectionHubContext struct {
	connection hubConnection
	clients    HubClients
	groups     GroupManager
}

func (c *connectionHubContext) Clients() HubClients {
	return c.clients
}

func (c *connectionHubContext) Groups() GroupManager {
	return c.groups
}

func (c *connectionHubContext) Items() *sync.Map {
	return c.connection.Items()
}

func (c *connectionHubContext) ConnectionID() string {
	return c.connection.ConnectionID()
}

func (c *connectionHubContext) Abort() {
	c.connection.Abort()
}
