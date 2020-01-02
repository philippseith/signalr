package signalr

type HubContext interface {
	Clients() HubClients
	Groups() GroupManager
}

type defaultHubContext struct {
	clients HubClients
	groups  GroupManager
}

func (d *defaultHubContext) Clients() HubClients {
	return d.clients
}

func (d *defaultHubContext) Groups() GroupManager {
	return d.groups
}
