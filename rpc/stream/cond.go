package stream

import (
	"context"
	"sync"
)

// Cond implements conditional variable with a channel
type Cond struct {
	mu sync.Mutex // guards ch
	ch chan struct{}
}

func NewCond() *Cond {
	return &Cond{ch: make(chan struct{})}
}

// Wait releases l, waits until the condition is signaled or the context is
// canceled, and acquires l again before it returns. The caller must hold l.
// The channel that Broadcast closes is taken before l is released, so a
// Broadcast made under the matching write lock after the caller checked its
// condition is not missed.
// It returns true if the condition is signaled, false if the context is canceled.
func (c *Cond) Wait(ctx context.Context, l sync.Locker) bool {
	c.mu.Lock()
	ch := c.ch
	c.mu.Unlock()

	l.Unlock()
	defer l.Lock()

	select {
	case <-ch:
		return true
	case <-ctx.Done():
		return false
	}
}

func (c *Cond) Broadcast() {
	c.mu.Lock()
	defer c.mu.Unlock()
	close(c.ch)
	c.ch = make(chan struct{})
}
