package signalr

import (
	"context"
	"io"
	"time"
)

// Connection describes a connection between signalR client and Server
type Connection interface {
	io.Reader
	io.Writer
	Context() context.Context
	ConnectionID() string
	SetConnectionID(id string)
	Timeout() time.Duration
	SetTimeout(duration time.Duration)
}
