package signalr

import (
	"context"
	"sync"
)

// HubInterface is a hubs interface
type HubInterface interface {
	Initialize(hubContext HubContext)
	OnConnected(connectionID string)
	OnDisconnected(connectionID string)
}

// Hub is a base class for hubs
type Hub struct {
	context HubContext
	cm      sync.RWMutex
}

// Initialize initializes a hub with a HubContext
func (h *Hub) Initialize(ctx HubContext) {
	h.cm.Lock()
	defer h.cm.Unlock()
	h.context = ctx
}

// Clients returns the clients of this hub
func (h *Hub) Clients() HubClients {
	h.cm.RLock()
	defer h.cm.RUnlock()
	return h.context.Clients()
}

// Groups returns the client groups of this hub
func (h *Hub) Groups() GroupManager {
	h.cm.RLock()
	defer h.cm.RUnlock()
	return h.context.Groups()
}

// Items returns the items for this connection
func (h *Hub) Items() *sync.Map {
	h.cm.RLock()
	defer h.cm.RUnlock()
	return h.context.Items()
}

// ConnectionID gets the ID of the current connection
func (h *Hub) ConnectionID() string {
	h.cm.RLock()
	defer h.cm.RUnlock()
	return h.context.ConnectionID()
}

// Context is the context.Context of the current connection
func (h *Hub) Context() context.Context {
	h.cm.RLock()
	defer h.cm.RUnlock()
	return h.context.Context()
}

// Abort aborts the current connection
func (h *Hub) Abort() {
	h.cm.RLock()
	defer h.cm.RUnlock()
	h.context.Abort()
}

// Logger returns the loggers used in this server. By this, derived hubs can use the same loggers as the server.
func (h *Hub) Logger() (info StructuredLogger, dbg StructuredLogger) {
	h.cm.RLock()
	defer h.cm.RUnlock()
	return h.context.Logger()
}

// OnConnected is called when the hub is connected
func (h *Hub) OnConnected(string) {}

// OnDisconnected is called when the hub is disconnected
func (h *Hub) OnDisconnected(string) {}
