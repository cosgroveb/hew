package cache

import (
	"testing"
	"time"
)

// DO NOT MODIFY THIS FILE.
// These tests pass with the current (buggy) code and must continue to pass
// after bug fixes. They exercise functionality not affected by the bugs.

// TestNewCache verifies that the constructor returns a usable cache.
func TestNewCache(t *testing.T) {
	c := New(10, time.Second)
	defer c.Stop()

	if c == nil {
		t.Fatal("New returned nil")
	}
	if c.Len() != 0 {
		t.Errorf("new cache should be empty, got Len() = %d", c.Len())
	}
}

// TestDelete verifies that deleting an entry removes it.
func TestDelete(t *testing.T) {
	c := New(10, time.Hour) // long eviction interval — evictor won't interfere
	defer c.Stop()

	c.Set("x", 42, time.Hour)

	// Delete should return true for existing key.
	if !c.Delete("x") {
		t.Error("Delete returned false for existing key")
	}

	// Delete should return false for missing key.
	if c.Delete("x") {
		t.Error("Delete returned true for already-deleted key")
	}

	if c.Len() != 0 {
		t.Errorf("expected empty cache after delete, got Len() = %d", c.Len())
	}
}

// TestStatsAccessible verifies that the Stats method returns a valid struct.
func TestStatsAccessible(t *testing.T) {
	c := New(10, time.Hour)
	defer c.Stop()

	stats := c.Stats()

	// Fresh cache should have all-zero stats.
	if stats.Hits != 0 || stats.Misses != 0 || stats.Evictions != 0 {
		t.Errorf("fresh cache stats should be zero, got %+v", stats)
	}
}

// TestSetOverwrite verifies that setting the same key twice overwrites the value.
func TestSetOverwrite(t *testing.T) {
	c := New(10, time.Hour)
	defer c.Stop()

	c.Set("dup", "first", time.Hour)
	c.Set("dup", "second", time.Hour)

	if c.Len() != 1 {
		t.Errorf("expected 1 entry after overwrite, got %d", c.Len())
	}
}

// TestDeleteNonexistent verifies that deleting a key that was never set is safe.
func TestDeleteNonexistent(t *testing.T) {
	c := New(10, time.Hour)
	defer c.Stop()

	if c.Delete("ghost") {
		t.Error("Delete returned true for key that was never set")
	}
}
