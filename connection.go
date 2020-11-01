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
	SetTimeout(duration time.Duration)
	Timeout() time.Duration
}
