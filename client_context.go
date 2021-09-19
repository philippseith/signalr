package signalr

import (
	"context"
	"sync"
	"time"
)

type clientContext struct {
	context.Context
	mx  sync.Mutex
	err error
}

func (c *clientContext) Err() error {
	defer c.mx.Unlock()
	c.mx.Lock()
	if c.err != nil {
		return c.err
	}
	return c.Err()
}

func (c *clientContext) SetErr(err error) {
	defer c.mx.Unlock()
	c.mx.Lock()
	c.err = err
}

func (c *clientContext) Deadline() (deadline time.Time, ok bool) {
	return c.Context.Deadline()
}

func (c *clientContext) Done() <-chan struct{} {
	return c.Context.Done()
}

func (c *clientContext) Value(key interface{}) interface{} {
	return c.Context.Value(key)
}
