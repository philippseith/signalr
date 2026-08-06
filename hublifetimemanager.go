package signalr

import (
	"sync"

	"github.com/go-kit/log"
)

// HubLifetimeManager is a lifetime manager abstraction for hub instances
// OnConnected() is called when a connection is started
// OnDisconnected() is called when a connection is finished
// InvokeAll() sends an invocation message to all hub connections
// InvokeClient() sends an invocation message to a specified hub connection
// InvokeGroup() sends an invocation message to a specified group of hub connections
// AddToGroup() adds a connection to the specified group
// RemoveFromGroup() removes a connection from the specified group
type HubLifetimeManager interface {
	OnConnected(conn HubConnection)
	// OnProtocolChange()
	OnDisconnected(conn HubConnection)
	InvokeAll(target string, args []interface{})
	InvokeClient(connectionID string, target string, args []interface{})
	InvokeGroup(groupName string, target string, args []interface{})
	AddToGroup(groupName, connectionID string)
	RemoveFromGroup(groupName, connectionID string)
}

func newLifeTimeManager(info StructuredLogger) defaultHubLifetimeManager {
	return defaultHubLifetimeManager{
		info: log.WithPrefix(info, "ts", log.DefaultTimestampUTC,
			"class", "lifeTimeManager"),
	}
}

type defaultHubLifetimeManager struct {
	clients sync.Map
	groups  sync.Map
	info    StructuredLogger
}

// groupMap is a mutex-protected map of HubConnections for one group.
type groupMap struct {
	mu      sync.RWMutex
	members map[string]HubConnection
}

func (g *groupMap) add(id string, conn HubConnection) {
	g.mu.Lock()
	g.members[id] = conn
	g.mu.Unlock()
}

func (g *groupMap) remove(id string) {
	g.mu.Lock()
	delete(g.members, id)
	g.mu.Unlock()
}

// snapshot returns a stable copy of the current members for safe iteration.
func (g *groupMap) snapshot() []HubConnection {
	g.mu.RLock()
	conns := make([]HubConnection, 0, len(g.members))
	for _, c := range g.members {
		conns = append(conns, c)
	}
	g.mu.RUnlock()
	return conns
}

func (d *defaultHubLifetimeManager) OnConnected(conn HubConnection) {
	d.clients.Store(conn.ConnectionID(), conn)
}

func (d *defaultHubLifetimeManager) OnDisconnected(conn HubConnection) {
	d.clients.Delete(conn.ConnectionID())
	d.groups.Range(func(_, value any) bool {
		value.(*groupMap).remove(conn.ConnectionID())
		return true
	})
}

func (d *defaultHubLifetimeManager) InvokeAll(target string, args []interface{}) {
	d.clients.Range(func(key, value interface{}) bool {
		go func() {
			_ = value.(HubConnection).SendInvocation("", target, args)
		}()
		return true
	})
}

func (d *defaultHubLifetimeManager) InvokeClient(connectionID string, target string, args []interface{}) {
	if client, ok := d.clients.Load(connectionID); ok {
		go func() {
			_ = client.(HubConnection).SendInvocation("", target, args)
		}()
	}
}

func (d *defaultHubLifetimeManager) InvokeGroup(groupName string, target string, args []interface{}) {
	if gm, ok := d.groups.Load(groupName); ok {
		for _, conn := range gm.(*groupMap).snapshot() {
			c := conn
			go func() {
				_ = c.SendInvocation("", target, args)
			}()
		}
	}
}

func (d *defaultHubLifetimeManager) AddToGroup(groupName string, connectionID string) {
	if client, ok := d.clients.Load(connectionID); ok {
		gm, _ := d.groups.LoadOrStore(groupName, &groupMap{members: make(map[string]HubConnection)})
		gm.(*groupMap).add(connectionID, client.(HubConnection))
	}
}

func (d *defaultHubLifetimeManager) RemoveFromGroup(groupName string, connectionID string) {
	if gm, ok := d.groups.Load(groupName); ok {
		gm.(*groupMap).remove(connectionID)
	}
}
