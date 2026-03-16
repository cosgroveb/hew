package ratelimiter

import "time"

// Config defines rate limiting parameters.
type Config struct {
	Rate   int           // Maximum requests allowed
	Window time.Duration // Time window for the rate
	Burst  int           // Maximum burst size (0 = same as Rate)
}

// Limiter controls request rates per key.
type Limiter interface {
	// Allow returns true if the request for the given key should be permitted.
	Allow(key string) bool
	// Reset clears rate limiting state for the given key.
	Reset(key string)
}

// Clock abstracts time for testing.
type Clock interface {
	Now() time.Time
}

// NewLimiter creates a rate limiter with the given configuration and clock.
// If clock is nil, use real time.
func NewLimiter(cfg Config, clock Clock) Limiter {
	return newLimiter(cfg, clock)
}
