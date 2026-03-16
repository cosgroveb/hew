package ratelimiter

import (
	"sync"
	"time"
)

// TestClock is a controllable clock for deterministic testing.
type TestClock struct {
	mu  sync.Mutex
	now time.Time
}

func NewTestClock(start time.Time) *TestClock {
	return &TestClock{now: start}
}

func (c *TestClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *TestClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}
