package signalr

//ClientProxy allows the hub to send messages to one or more of its clients
type ClientProxy interface {
	Send(target string, args ...interface{})
}

type allClientProxy struct {
	lifetimeManager HubLifetimeManager
}

func (a *allClientProxy) Send(target string, args ...interface{}) {
	a.lifetimeManager.InvokeAll(target, args)
}

type singleClientProxy struct {
	connectionID    string
	lifetimeManager HubLifetimeManager
}

func (a *singleClientProxy) Send(target string, args ...interface{}) {
	a.lifetimeManager.InvokeClient(a.connectionID, target, args)
}

type groupClientProxy struct {
	groupName       string
	lifetimeManager HubLifetimeManager
}

func (g *groupClientProxy) Send(target string, args ...interface{}) {
	g.lifetimeManager.InvokeGroup(g.groupName, target, args)
}
