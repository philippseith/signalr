package signalr

type hubConnection interface {
	isConnected() bool
	getConnectionID() string
	receive() (interface{}, error)
	sendInvocation(target string, args []interface{})
	streamItem(id string, item interface{})
	completion(id string, result interface{}, error string)
	ping()
}

