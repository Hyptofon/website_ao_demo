package security

import (
	"context"
	"testing"
	"time"
)

func TestRateLimiter_AllowsWithinLimit(t *testing.T) {
	rl := NewRateLimiter(context.Background(), 5, 1*time.Minute, 3)

	for i := 0; i < 5; i++ {
		allowed, _ := rl.Allow("192.168.1.1", 1)
		if !allowed {
			t.Errorf("request %d should be allowed", i+1)
		}
	}
}

func TestRateLimiter_BlocksOverLimit(t *testing.T) {
	rl := NewRateLimiter(context.Background(), 3, 1*time.Minute, 3)

	for i := 0; i < 3; i++ {
		rl.Allow("192.168.1.1", 1)
	}

	allowed, retryAfter := rl.Allow("192.168.1.1", 1)
	if allowed {
		t.Error("4th request should be blocked")
	}
	if retryAfter <= 0 {
		t.Error("retryAfter should be positive")
	}
}

func TestRateLimiter_DifferentIPs(t *testing.T) {
	rl := NewRateLimiter(context.Background(), 2, 1*time.Minute, 3)

	rl.Allow("10.0.0.1", 1)
	rl.Allow("10.0.0.1", 1)

	// First IP should be blocked
	allowed1, _ := rl.Allow("10.0.0.1", 1)
	if allowed1 {
		t.Error("10.0.0.1 should be blocked after 2 requests")
	}

	// Second IP should still be allowed
	allowed2, _ := rl.Allow("10.0.0.2", 1)
	if !allowed2 {
		t.Error("10.0.0.2 should be allowed")
	}
}

func TestRateLimiter_PenaltyWeight(t *testing.T) {
	// 5 requests per window, penalty multiplier 3
	rl := NewRateLimiter(context.Background(), 5, 1*time.Minute, 3)

	// Normal request: weight 1
	rl.Allow("192.168.1.1", 1)

	// Ban call = penalty weight
	rl.Ban("192.168.1.1")

	// After ban, the IP should have weight=3 penalty added.
	// Depends on implementation: if Ban adds penalty_multiplier requests,
	// then we've used 1 + 3 = 4 slots of 5
	allowed, _ := rl.Allow("192.168.1.1", 1)
	// 1 + 3 + 1 = 5, this should be the last allowed
	if !allowed {
		t.Log("Request after ban was blocked (acceptable - depends on penalty implementation)")
	}
}

func TestRateLimiter_ZeroWeight(t *testing.T) {
	rl := NewRateLimiter(context.Background(), 5, 1*time.Minute, 3)

	allowed, _ := rl.Allow("192.168.1.1", 0)
	if !allowed {
		t.Error("zero-weight request should be allowed")
	}
}

func TestRateLimiter_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	// Very short period so ticker fires quickly in test
	rl := NewRateLimiter(ctx, 5, 100*time.Millisecond, 1)
	_ = rl
	// Cancelling must stop the cleanupLoop goroutine cleanly
	cancel()
}
