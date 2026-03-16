package cache

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// DO NOT MODIFY THIS FILE.
// These tests expose bugs in cache.go. Fix the bugs, not the tests.

// TestSetAndGet verifies that entries survive their TTL.
// Fails because the TTL calculation is inverted, causing immediate expiry.
func TestSetAndGet(t *testing.T) {
	c := New(100, 50*time.Millisecond)
	defer c.Stop()

	c.Set("alpha", "one", 2*time.Second)
	c.Set("beta", "two", 2*time.Second)
	c.Set("gamma", "three", 2*time.Second)

	// Entries should still be alive — they have a 2-second TTL.
	for _, tc := range []struct {
		key  string
		want string
	}{
		{"alpha", "one"},
		{"beta", "two"},
		{"gamma", "three"},
	} {
		val, ok := c.Get(tc.key)
		if !ok {
			t.Errorf("Get(%q): expected hit, got miss", tc.key)
			continue
		}
		if val.(string) != tc.want {
			t.Errorf("Get(%q) = %v, want %v", tc.key, val, tc.want)
		}
	}

	stats := c.Stats()
	if stats.Hits != 3 {
		t.Errorf("expected 3 hits, got %d", stats.Hits)
	}
	if stats.Misses != 0 {
		t.Errorf("expected 0 misses, got %d", stats.Misses)
	}
}

// TestConcurrentAccess hammers the cache from multiple goroutines.
// Fails with -race because the background evictor reads without a lock.
func TestConcurrentAccess(t *testing.T) {
	c := New(500, 5*time.Millisecond)
	defer c.Stop()

	var wg sync.WaitGroup
	const workers = 10
	const opsPerWorker = 200

	// Writers
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < opsPerWorker; i++ {
				key := fmt.Sprintf("w%d-k%d", id, i)
				c.Set(key, i, 50*time.Millisecond)
			}
		}(w)
	}

	// Readers
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < opsPerWorker; i++ {
				key := fmt.Sprintf("w%d-k%d", id, i)
				c.Get(key)
			}
		}(w)
	}

	wg.Wait()

	// Verify stats are consistent: total operations = hits + misses + some evictions.
	stats := c.Stats()
	totalLookups := stats.Hits + stats.Misses
	expectedLookups := int64(workers * opsPerWorker)
	if totalLookups != expectedLookups {
		t.Errorf("expected %d total lookups (hits+misses), got %d (hits=%d, misses=%d)",
			expectedLookups, totalLookups, stats.Hits, stats.Misses)
	}
}

// TestMaxSizeEviction verifies that the cache respects its size limit.
// Fails with a panic or incorrect eviction count due to the race between
// the background evictor and the LRU eviction in Set().
func TestMaxSizeEviction(t *testing.T) {
	const maxSize = 5
	c := New(maxSize, 10*time.Millisecond)
	defer c.Stop()

	// Insert more entries than the cache can hold.
	for i := 0; i < 20; i++ {
		key := fmt.Sprintf("key-%d", i)
		c.Set(key, i, 2*time.Second)
	}

	// Cache should not exceed max size.
	if n := c.Len(); n > maxSize {
		t.Errorf("cache size %d exceeds max %d", n, maxSize)
	}

	// The most recently inserted entries should be present.
	for i := 15; i < 20; i++ {
		key := fmt.Sprintf("key-%d", i)
		val, ok := c.Get(key)
		if !ok {
			t.Errorf("Get(%q): expected hit for recent entry, got miss", key)
			continue
		}
		if val.(int) != i {
			t.Errorf("Get(%q) = %v, want %d", key, val, i)
		}
	}
}
