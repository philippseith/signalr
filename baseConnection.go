package signalr

import (
	"context"
	"time"
)

type cancelableConnection interface {
	Connection
	initCancel(ctx context.Context)
	context() context.Context
	cancel()
}

type baseConnection struct {
	ctx          context.Context
	cancelFunc   context.CancelFunc
	connectionID string
	timeout      time.Duration
}

func (b *baseConnection) initCancel(ctx context.Context) {
	b.ctx, b.cancelFunc = context.WithCancel(ctx)
}

func (b *baseConnection) context() context.Context {
	return b.ctx
}

func (b *baseConnection) cancel() {
	b.cancelFunc()
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
