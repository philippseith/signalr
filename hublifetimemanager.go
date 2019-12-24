package signalr

import "sync"

type HubLifetimeManager interface {
	OnConnected(conn HubConnection)
	OnDisconnected(conn HubConnection)
	InvokeAll(target string, args []interface{})
	InvokeClient(connectionID string, target string, args []interface{})
	InvokeGroup(groupName string, target string, args []interface{})
	AddToGroup(groupName, connectionID string)
	RemoveFromGroup(groupName, connectionID string)
}

type defaultHubLifetimeManager struct {
	clients sync.Map
	groups  sync.Map
}

func (d *defaultHubLifetimeManager) OnConnected(conn HubConnection) {
	d.clients.Store(conn.GetConnectionID(), conn)
}

func (d *defaultHubLifetimeManager) OnDisconnected(conn HubConnection) {
	d.clients.Delete(conn.GetConnectionID())
}

func (d *defaultHubLifetimeManager) InvokeAll(target string, args []interface{}) {
	d.clients.Range(func(key, value interface{}) bool {
		value.(HubConnection).SendInvocation(target, args)
		return true
	})
}

func (d *defaultHubLifetimeManager) InvokeClient(connectionID string, target string, args []interface{}) {
	client, ok := d.clients.Load(connectionID)

	if !ok {
		return
	}

	client.(HubConnection).SendInvocation(target, args)
}

func (d *defaultHubLifetimeManager) InvokeGroup(groupName string, target string, args []interface{}) {
	groups, ok := d.groups.Load(groupName)

	if !ok {
		// No such group
		return
	}

	for _, v := range groups.(map[string]HubConnection) {
		v.SendInvocation(target, args)
	}
}

func (d *defaultHubLifetimeManager) AddToGroup(groupName string, connectionID string) {
	client, ok := d.clients.Load(connectionID)

	if !ok {
		// No such client
		return
	}

	groups, _ := d.groups.LoadOrStore(groupName, make(map[string]HubConnection))

	groups.(map[string]HubConnection)[connectionID] = client.(HubConnection)
}

func (d *defaultHubLifetimeManager) RemoveFromGroup(groupName string, connectionID string) {
	groups, ok := d.groups.Load(groupName)

	if !ok {
		return
	}

	delete(groups.(map[string]HubConnection), connectionID)
}



