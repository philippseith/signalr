package signalr

// GroupManager manages the client groups of the hub
type GroupManager interface {
	AddToGroup(groupName string, connectionID string)
	RemoveFromGroup(groupName string, connectionID string)
}

type defaultGroupManager struct {
	lifetimeManager HubLifetimeManager
}

func (d *defaultGroupManager) AddToGroup(groupName string, connectionID string) {
	d.lifetimeManager.AddToGroup(groupName, connectionID)
}

func (d *defaultGroupManager) RemoveFromGroup(groupName string, connectionID string) {
	d.lifetimeManager.RemoveFromGroup(groupName, connectionID)
}
