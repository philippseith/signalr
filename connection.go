package signalr

import "io"

// Connection describes a connection between signalR client and Server
type Connection interface {
	io.Reader
	io.Writer
	ConnectionID() string
}
