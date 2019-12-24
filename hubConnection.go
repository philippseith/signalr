package signalr

type HubConnection interface {
	IsConnected() bool
	GetConnectionID() string
	Receive() (interface{}, error)
	SendInvocation(target string, args []interface{})
	StreamItem(id string, item interface{})
	Completion(id string, result interface{}, error string)
	Ping()
}

