package signalr

type HubClients interface {
	All() ClientProxy
	Client(connectionID string) ClientProxy
	Group(groupName string) ClientProxy
	Caller() ClientProxy
}

type defaultHubClients struct {
	lifetimeManager HubLifetimeManager
	allCache        allClientProxy
}

func (c *defaultHubClients) All() ClientProxy {
	return &c.allCache
}

func (c *defaultHubClients) Client(connectionID string) ClientProxy {
	return &singleClientProxy{connectionID: connectionID, lifetimeManager: c.lifetimeManager}
}

func (c *defaultHubClients) Group(groupName string) ClientProxy {
	return &groupClientProxy{groupName: groupName, lifetimeManager: c.lifetimeManager}
}

func (c *defaultHubClients) Caller() ClientProxy {
	panic("use only with contextHubClients")
}

type contextHubClients struct {
	defaultHubClients HubClients
	connectionID      string
}

func (c *contextHubClients) All() ClientProxy {
	return c.defaultHubClients.All()
}

func (c *contextHubClients) Client(connectionID string) ClientProxy {
	return c.defaultHubClients.Client(connectionID)
}

func (c *contextHubClients) Group(groupName string) ClientProxy {
	return c.defaultHubClients.Group(groupName)
}

func (c *contextHubClients) Caller() ClientProxy {
	return c.defaultHubClients.Client(c.connectionID)
}

