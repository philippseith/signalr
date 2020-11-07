package signalr

import (
	"context"
	"time"
)

type baseConnection struct {
	ctx          context.Context
	connectionID string
	timeout      time.Duration
}

func (b *baseConnection) Context() context.Context {
	return b.ctx
}

func (b *baseConnection) ConnectionID() string {
	return b.connectionID
}

func (b *baseConnection) SetTimeout(duration time.Duration) {
	b.timeout = duration
}

func (b *baseConnection) Timeout() time.Duration {
	return b.timeout
}
