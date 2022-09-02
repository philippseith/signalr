package signalr

type HubProxy interface {
	CanInvoke(invocation invocationMessage) bool
	Invoke(l *loop, invocation invocationMessage)
}

type sampleHub struct{}

func (h *sampleHub) CallMe(call string) string {
	return call + call
}

type sampleProxy struct {
	hub *sampleHub
}

func (s *sampleProxy) CanInvoke(invocation invocationMessage) bool {
	switch invocation.Target {
	case "CallMe":
		return true
	}
	return false
}

func (s *sampleProxy) Invoke(l *loop, invocation invocationMessage) {
	l.hubConn.Completion(invocation.InvocationID,
		s.hub.CallMe(invocation.Arguments[0].(string)),
		"")
}
