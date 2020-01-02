package signalr

type HubInterface interface {
	Initialize(hubContext HubContext)
	OnConnected(connectionID string)
	OnDisconnected(connectionID string)
}

type Hub struct {
	context HubContext
}

func (h *Hub) Initialize(ctx HubContext) {
	h.context = ctx
}

func (h *Hub) Clients() HubClients {
	return h.context.Clients()
}

func (h *Hub) Groups() GroupManager {
	return h.context.Groups()
}

func (h *Hub) OnConnected(string) {}

func (h *Hub) OnDisconnected(string) {}
