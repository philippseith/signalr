package signalr

// HubContext is a context abstraction for a hub
// Clients() gets a HubClients that can be used to invoke methods on clients connected to the hub
// Groups() gets a GroupManager that can be used to add and remove connections to named groups
// Items() holds key/value pairs scoped to the hubs connection
type HubContext interface {
	Clients() HubClients
	Groups() GroupManager
	Items() map[string]interface{}
}

type connectionHubContext struct {
	clients HubClients
	groups  GroupManager
	items   map[string]interface{}
}

func (c *connectionHubContext) Clients() HubClients {
	return c.clients
}

func (c *connectionHubContext) Groups() GroupManager {
	return c.groups
}

func (c *connectionHubContext) Items() map[string]interface{} {
	return c.items
}
