package signalr

import "sync"

// HubInterface is a hubs interface
type HubInterface interface {
	Initialize(hubContext HubContext)
	OnConnected(connectionID string)
	OnDisconnected(connectionID string)
}

// Hub is a base class for hubs
type Hub struct {
	context HubContext
}

// Initialize initializes a hub with a HubContext
func (h *Hub) Initialize(ctx HubContext) {
	h.context = ctx
}

// Clients returns the clients of this hub
func (h *Hub) Clients() HubClients {
	return h.context.Clients()
}

// Groups returns the client groups of this hub
func (h *Hub) Groups() GroupManager {
	return h.context.Groups()
}

// Items returns the items for this connection
func (h *Hub) Items() *sync.Map {
	return h.context.Items()
}

// OnConnected is called when the hub is connected
func (h *Hub) OnConnected(string) {}

//OnDisconnected is called when the hub is disconnected
func (h *Hub) OnDisconnected(string) {}
