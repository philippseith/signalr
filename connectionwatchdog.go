package signalr

import (
	"context"
	"sync"
	"time"
)

type connectionWatchDogQueue struct {
	mx      sync.Mutex
	dog     *connectionWatchDog
	dogChan chan *connectionWatchDog
}

func newConnectionWatchDogQueue() connectionWatchDogQueue {
	return connectionWatchDogQueue{dogChan: make(chan *connectionWatchDog, 1)}
}

// Run starts a queue of watchdogs for Read and Write. The active connectionWatchdog stops the connection
// when its timeout has elapsed. If ChangeGuard is called before the timeout of the active dog has elapsed, it will
// be replaced, and it will not cancel its context when the timeout is over.
// The new dog will start waiting for the new timeout. If timeout is set to 0, it will not wait at all.
func (q *connectionWatchDogQueue) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case newDog := <-q.dogChan:
			q.mx.Lock()
			if q.dog != nil {
				if q.dog.timer != nil && !q.dog.timer.Stop() {
					go func(c <-chan time.Time) {
						<-c
					}(q.dog.timer.C)
				}
				q.dog.Cancel()
			}
			q.dog = newDog
			if q.dog != nil {
				go q.dog.BarkOrDie()
			}
			q.mx.Unlock()
		}
	}
}

// ChangeGuard changes the active watchdog for Read and Write.
func (q *connectionWatchDogQueue) ChangeGuard(ctx context.Context, timeout time.Duration) context.Context {
	if timeout > 0 {
		var dog *connectionWatchDog
		ctx, dog = newWatchDog(ctx, timeout)
		q.dogChan <- dog
	} else {
		q.dogChan <- nil
	}
	return ctx
}

type connectionWatchDog struct {
	// After this, the dog will bark
	timer      *time.Timer
	cancelChan chan struct{}
	bark       context.CancelFunc
}

func newWatchDog(ctx context.Context, timeout time.Duration) (context.Context, *connectionWatchDog) {
	dog := &connectionWatchDog{
		timer:      time.NewTimer(timeout),
		cancelChan: make(chan struct{}),
	}
	var dogCtx context.Context
	dogCtx, dog.bark = context.WithCancel(ctx)
	return dogCtx, dog
}

func (d *connectionWatchDog) Cancel() {
	close(d.cancelChan)
}

func (d *connectionWatchDog) BarkOrDie() {
	select {
	case <-d.cancelChan:
	case <-d.timer.C:
		d.bark()
	}
}
