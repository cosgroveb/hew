package ratelimiter

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var epoch = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

func TestBasicLimiting(t *testing.T) {
	clock := NewTestClock(epoch)
	lim := NewLimiter(Config{Rate: 5, Window: time.Second}, clock)

	for i := 0; i < 5; i++ {
		if !lim.Allow("user1") {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}

	if lim.Allow("user1") {
		t.Fatal("request 6 should be denied after rate limit reached")
	}
	if lim.Allow("user1") {
		t.Fatal("request 7 should also be denied")
	}
}

func TestWindowExpiry(t *testing.T) {
	clock := NewTestClock(epoch)
	lim := NewLimiter(Config{Rate: 3, Window: time.Second}, clock)

	// Exhaust the limit.
	for i := 0; i < 3; i++ {
		if !lim.Allow("k") {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	if lim.Allow("k") {
		t.Fatal("should be denied after exhausting rate")
	}

	// Advance past the window.
	clock.Advance(time.Second)

	// Should be allowed again.
	for i := 0; i < 3; i++ {
		if !lim.Allow("k") {
			t.Fatalf("request %d after window reset should be allowed", i+1)
		}
	}
	if lim.Allow("k") {
		t.Fatal("should be denied again after exhausting second window")
	}
}

func TestKeyIsolation(t *testing.T) {
	clock := NewTestClock(epoch)
	lim := NewLimiter(Config{Rate: 2, Window: time.Second}, clock)

	// Exhaust key "a".
	lim.Allow("a")
	lim.Allow("a")
	if lim.Allow("a") {
		t.Fatal("key 'a' should be denied")
	}

	// Key "b" should be independent.
	if !lim.Allow("b") {
		t.Fatal("key 'b' should be allowed (independent of 'a')")
	}
	if !lim.Allow("b") {
		t.Fatal("key 'b' second request should be allowed")
	}
	if lim.Allow("b") {
		t.Fatal("key 'b' third request should be denied")
	}
}

func TestReset(t *testing.T) {
	clock := NewTestClock(epoch)
	lim := NewLimiter(Config{Rate: 2, Window: time.Second}, clock)

	// Exhaust key "x".
	lim.Allow("x")
	lim.Allow("x")
	if lim.Allow("x") {
		t.Fatal("should be denied after exhausting rate")
	}

	// Exhaust key "y".
	lim.Allow("y")
	lim.Allow("y")

	// Reset only "x".
	lim.Reset("x")

	// "x" should be allowed again.
	if !lim.Allow("x") {
		t.Fatal("key 'x' should be allowed after reset")
	}
	if !lim.Allow("x") {
		t.Fatal("key 'x' second request should be allowed after reset")
	}

	// "y" should still be denied (not reset).
	if lim.Allow("y") {
		t.Fatal("key 'y' should still be denied (was not reset)")
	}
}

func TestBurstAboveRate(t *testing.T) {
	clock := NewTestClock(epoch)
	lim := NewLimiter(Config{Rate: 2, Window: time.Second, Burst: 5}, clock)

	// Should allow up to Burst requests initially.
	for i := 0; i < 5; i++ {
		if !lim.Allow("u") {
			t.Fatalf("burst request %d should be allowed", i+1)
		}
	}
	if lim.Allow("u") {
		t.Fatal("should be denied after burst exhausted")
	}

	// After one window, should recover Rate tokens (not Burst).
	clock.Advance(time.Second)

	allowed := 0
	for i := 0; i < 5; i++ {
		if lim.Allow("u") {
			allowed++
		}
	}
	if allowed != 2 {
		t.Fatalf("after one window, expected 2 allowed (Rate), got %d", allowed)
	}
}

func TestBurstDefaultEqualsRate(t *testing.T) {
	clock := NewTestClock(epoch)
	lim := NewLimiter(Config{Rate: 4, Window: time.Second, Burst: 0}, clock)

	// With Burst=0, effective burst should equal Rate.
	for i := 0; i < 4; i++ {
		if !lim.Allow("k") {
			t.Fatalf("request %d should be allowed (burst defaults to rate)", i+1)
		}
	}
	if lim.Allow("k") {
		t.Fatal("request 5 should be denied")
	}
}

func TestConcurrentAccess(t *testing.T) {
	clock := NewTestClock(epoch)
	lim := NewLimiter(Config{Rate: 100, Window: time.Second}, clock)

	var wg sync.WaitGroup
	var allowed atomic.Int64

	// Fire 200 concurrent requests on the same key.
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if lim.Allow("shared") {
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()

	got := allowed.Load()
	if got != 100 {
		t.Fatalf("expected exactly 100 allowed, got %d", got)
	}
}

func TestMultipleWindows(t *testing.T) {
	clock := NewTestClock(epoch)
	lim := NewLimiter(Config{Rate: 3, Window: 500 * time.Millisecond}, clock)

	for window := 0; window < 4; window++ {
		for i := 0; i < 3; i++ {
			if !lim.Allow("w") {
				t.Fatalf("window %d, request %d should be allowed", window, i+1)
			}
		}
		if lim.Allow("w") {
			t.Fatalf("window %d: request 4 should be denied", window)
		}
		clock.Advance(500 * time.Millisecond)
	}
}

func TestHighRate(t *testing.T) {
	clock := NewTestClock(epoch)
	lim := NewLimiter(Config{Rate: 1000, Window: time.Second}, clock)

	for i := 0; i < 1000; i++ {
		if !lim.Allow("hi") {
			t.Fatalf("request %d of 1000 should be allowed", i+1)
		}
	}
	if lim.Allow("hi") {
		t.Fatal("request 1001 should be denied")
	}

	// Advance half a window -- should still be denied with a correct
	// token bucket (tokens replenish proportionally) or fixed/sliding window.
	// We allow either behavior: some algorithms refill partially, others
	// require the full window. So we just verify that after a full window
	// the limiter works again.
	clock.Advance(time.Second)

	allowed := 0
	for i := 0; i < 1000; i++ {
		if lim.Allow("hi") {
			allowed++
		}
	}
	if allowed != 1000 {
		t.Fatalf("after full window, expected 1000 allowed, got %d", allowed)
	}
}

func TestPartialWindowRecovery(t *testing.T) {
	clock := NewTestClock(epoch)
	lim := NewLimiter(Config{Rate: 10, Window: time.Second}, clock)

	// Use all 10 tokens.
	for i := 0; i < 10; i++ {
		lim.Allow("p")
	}
	if lim.Allow("p") {
		t.Fatal("should be denied after exhausting rate")
	}

	// Advance by a full window and verify full recovery.
	clock.Advance(time.Second)

	allowed := 0
	for i := 0; i < 10; i++ {
		if lim.Allow("p") {
			allowed++
		}
	}
	if allowed != 10 {
		t.Fatalf("expected 10 allowed after full window, got %d", allowed)
	}
}

func TestBurstRecoveryOverMultipleWindows(t *testing.T) {
	clock := NewTestClock(epoch)
	// Rate=2/sec, Burst=6. After exhausting burst, each window adds 2 tokens.
	lim := NewLimiter(Config{Rate: 2, Window: time.Second, Burst: 6}, clock)

	// Exhaust all 6 burst tokens.
	for i := 0; i < 6; i++ {
		if !lim.Allow("b") {
			t.Fatalf("burst request %d should be allowed", i+1)
		}
	}
	if lim.Allow("b") {
		t.Fatal("should be denied after burst exhausted")
	}

	// After 1 window: recover 2 tokens.
	clock.Advance(time.Second)
	for i := 0; i < 2; i++ {
		if !lim.Allow("b") {
			t.Fatalf("window 1 request %d should be allowed", i+1)
		}
	}
	if lim.Allow("b") {
		t.Fatal("window 1: should be denied after 2 requests")
	}

	// After 3 more windows: recover 6 tokens (capped at burst=6).
	clock.Advance(3 * time.Second)
	allowed := 0
	for i := 0; i < 8; i++ {
		if lim.Allow("b") {
			allowed++
		}
	}
	if allowed != 6 {
		t.Fatalf("after 3 windows, expected 6 allowed (burst cap), got %d", allowed)
	}
}

func TestResetDuringWindow(t *testing.T) {
	clock := NewTestClock(epoch)
	lim := NewLimiter(Config{Rate: 5, Window: time.Second}, clock)

	// Use 3 of 5.
	for i := 0; i < 3; i++ {
		lim.Allow("r")
	}

	// Advance partway into the window.
	clock.Advance(300 * time.Millisecond)

	// Reset mid-window.
	lim.Reset("r")

	// Should have full allowance again.
	for i := 0; i < 5; i++ {
		if !lim.Allow("r") {
			t.Fatalf("request %d after mid-window reset should be allowed", i+1)
		}
	}
	if lim.Allow("r") {
		t.Fatal("should be denied after exhausting rate post-reset")
	}
}

func TestManyKeys(t *testing.T) {
	clock := NewTestClock(epoch)
	lim := NewLimiter(Config{Rate: 1, Window: time.Second}, clock)

	// Each of 50 keys gets exactly 1 request.
	for i := 0; i < 50; i++ {
		key := fmt.Sprintf("key-%d", i)
		if !lim.Allow(key) {
			t.Fatalf("first request for %s should be allowed", key)
		}
		if lim.Allow(key) {
			t.Fatalf("second request for %s should be denied", key)
		}
	}
}
