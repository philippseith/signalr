package signalr

import (
	"io"
	"time"
)

// Connection describes a connection between signalR client and Server
type Connection interface {
	io.Reader
	io.Writer
	ConnectionID() string
	SetTimeout(duration time.Duration)
	Timeout() time.Duration
}
