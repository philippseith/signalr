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
	cm      sync.Mutex
}

// Initialize initializes a hub with a HubContext
func (h *Hub) Initialize(ctx HubContext) {
	defer h.cm.Unlock()
	h.cm.Lock()
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

// Logger returns the loggers used in this server. By this, derived hubs can use the same loggers as the server.
func (h *Hub) Logger() (info StructuredLogger, dbg StructuredLogger) {
	return h.context.Logger()
}

// OnConnected is called when the hub is connected
func (h *Hub) OnConnected(string) {}

//OnDisconnected is called when the hub is disconnected
func (h *Hub) OnDisconnected(string) {}
