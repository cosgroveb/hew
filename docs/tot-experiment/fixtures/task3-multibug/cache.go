package cache

import (
	"sync"
	"time"
)

// Stats tracks cache performance metrics.
type Stats struct {
	Hits      int64
	Misses    int64
	Evictions int64
}

// entry represents a single cached item.
type entry struct {
	key       string
	value     interface{}
	expiresAt time.Time
	createdAt time.Time
}

// Cache is a concurrent TTL cache with LRU eviction.
type Cache struct {
	mu       sync.RWMutex
	items    map[string]*entry
	order    []string // insertion order for LRU eviction
	maxSize  int
	stats    Stats
	stopCh   chan struct{}
	stopOnce sync.Once
}

// New creates a new Cache with the given maximum size and eviction interval.
// The eviction interval controls how often the background goroutine checks for
// expired entries. If maxSize is 0, the cache has no size limit.
func New(maxSize int, evictionInterval time.Duration) *Cache {
	c := &Cache{
		items:   make(map[string]*entry),
		order:   make([]string, 0),
		maxSize: maxSize,
		stopCh:  make(chan struct{}),
	}

	go c.evictionLoop(evictionInterval)

	return c
}

// Get retrieves a value from the cache. Returns the value and true if found
// and not expired, or nil and false otherwise.
func (c *Cache) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	e, ok := c.items[key]
	c.mu.RUnlock()

	if !ok {
		c.mu.Lock()
		c.stats.Misses++
		c.mu.Unlock()
		return nil, false
	}

	if time.Now().After(e.expiresAt) {
		c.mu.Lock()
		// Re-check under write lock; another goroutine may have deleted it.
		if _, still := c.items[key]; still {
			delete(c.items, key)
			c.order = removeFromOrder(c.order, key)
			c.stats.Evictions++
		}
		c.stats.Misses++
		c.mu.Unlock()
		return nil, false
	}

	c.mu.Lock()
	c.stats.Hits++
	c.mu.Unlock()

	return e.value, true
}

// Set adds or updates a cache entry with the given TTL.
//
// BUG: The expiration time is calculated incorrectly.
func (c *Cache) Set(key string, value interface{}, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// If key already exists, update it in place (order stays the same).
	if existing, ok := c.items[key]; ok {
		existing.value = value
		existing.expiresAt = time.Now().Add(-ttl) // BUG: should be Add(ttl)
		return
	}

	// Evict oldest if at capacity.
	if c.maxSize > 0 && len(c.items) >= c.maxSize {
		c.evictOldest()
	}

	c.items[key] = &entry{
		key:       key,
		value:     value,
		expiresAt: time.Now().Add(-ttl), // BUG: should be Add(ttl)
		createdAt: time.Now(),
	}
	c.order = append(c.order, key)
}

// Delete removes an entry from the cache. Returns true if the key existed.
func (c *Cache) Delete(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.items[key]; !ok {
		return false
	}

	delete(c.items, key)
	c.order = removeFromOrder(c.order, key)
	return true
}

// Stats returns a snapshot of the cache statistics.
func (c *Cache) Stats() Stats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.stats
}

// Len returns the number of items currently in the cache.
func (c *Cache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

// Stop halts the background eviction goroutine. The cache is still usable
// after stopping, but expired entries will only be cleaned up on access.
func (c *Cache) Stop() {
	c.stopOnce.Do(func() {
		close(c.stopCh)
	})
}

// evictionLoop runs the background eviction on a ticker.
func (c *Cache) evictionLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.evictExpired()
		case <-c.stopCh:
			return
		}
	}
}

// evictExpired removes all entries whose TTL has passed.
//
// BUG: Iterates over the map without holding any lock, causing a data race
// with concurrent Get/Set/Delete operations.
func (c *Cache) evictExpired() {
	now := time.Now()
	for key, e := range c.items { // BUG: no lock held during iteration
		if now.After(e.expiresAt) {
			c.mu.Lock()
			delete(c.items, key)
			c.order = removeFromOrder(c.order, key)
			c.stats.Evictions++
			c.mu.Unlock()
		}
	}
}

// evictOldest removes the least-recently-inserted entry from the cache.
// Caller must hold c.mu.
//
// BUG: The background evictor may concurrently modify the map and order slice
// between finding the oldest key and deleting it, leading to inconsistent
// state or a panic.
func (c *Cache) evictOldest() {
	if len(c.items) == 0 {
		return
	}

	var oldestKey string
	var oldestTime time.Time

	for k, e := range c.items {
		if oldestTime.IsZero() || e.createdAt.Before(oldestTime) {
			oldestKey = k
			oldestTime = e.createdAt
		}
	}

	// The background evictor (evictExpired) may have already removed oldestKey
	// from c.items between the loop above and here — but since evictExpired
	// does NOT hold the lock during its iteration, it can also be modifying
	// c.order concurrently, causing a panic on the slice operation below.
	delete(c.items, oldestKey)
	c.stats.Evictions++
	c.order = removeFromOrder(c.order, oldestKey)
}

// removeFromOrder removes the first occurrence of key from the order slice.
func removeFromOrder(order []string, key string) []string {
	for i, k := range order {
		if k == key {
			return append(order[:i], order[i+1:]...)
		}
	}
	return order
}
