package signalr

//HubContext holds the clients and groups connected to the hub
type HubContext interface {
	Clients() HubClients
	Groups() GroupManager
	Items() map[string]interface{}
}

type connectionHubContext struct {
	clients HubClients
	groups  GroupManager
	items map[string]interface{}
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
