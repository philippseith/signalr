package signalr

// HubClients gives the hub access to various client groups
// All() gets a ClientProxy that can be used to invoke methods on all clients connected to the hub
// Caller() gets a ClientProxy that can be used to invoke methods of the current calling client
// Client() gets a ClientProxy that can be used to invoke methods on the specified client connection
// Group() gets a ClientProxy that can be used to invoke methods on all connections in the specified group
type HubClients interface {
	All() ClientProxy
	Caller() ClientProxy
	Client(connectionID string) ClientProxy
	Group(groupName string) ClientProxy
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

// Caller is only implemented to fulfill the HubClients interface, so the servers defaultHubClients interface can be
// used for implementing Server.HubClients.
func (c *defaultHubClients) Caller() ClientProxy {
	return nil
}

type callerHubClients struct {
	defaultHubClients *defaultHubClients
	connectionID      string
}

func (c *callerHubClients) All() ClientProxy {
	return c.defaultHubClients.All()
}

func (c *callerHubClients) Caller() ClientProxy {
	return c.defaultHubClients.Client(c.connectionID)
}

func (c *callerHubClients) Client(connectionID string) ClientProxy {
	return c.defaultHubClients.Client(connectionID)
}

func (c *callerHubClients) Group(groupName string) ClientProxy {
	return c.defaultHubClients.Group(groupName)
}
