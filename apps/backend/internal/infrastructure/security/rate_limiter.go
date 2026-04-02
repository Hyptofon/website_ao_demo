package security

import (
	"sync"
	"time"
)

// window is a fixed-size ring-buffer of timestamps for one IP.
type window struct {
	mu        sync.Mutex
	ts        []time.Time
	banUntil  time.Time
}

// RateLimiter implements an in-memory Sliding Window rate limiter.
// No external DB required — uses sync.Map for thread-safe per-IP tracking.
type RateLimiter struct {
	mu          sync.Mutex
	windows     sync.Map   // map[string]*window
	limit       int        // max requests per period
	period      time.Duration
	penaltyMult int        // spam/off-topic request penalty multiplier
	banDuration time.Duration
}

// NewRateLimiter creates a limiter with the given parameters.
// limit: max requests per period
// period: sliding window size (e.g. 5 minutes)
// penaltyMult: how many "normal" requests a spam/off-topic counts as
func NewRateLimiter(limit int, period time.Duration, penaltyMult int) *RateLimiter {
	rl := &RateLimiter{
		limit:       limit,
		period:      period,
		penaltyMult: penaltyMult,
		banDuration: period,
	}
	// Background cleaner to prevent memory leak on idle goroutines.
	go rl.cleanupLoop()
	return rl
}

// Allow checks whether the IP is allowed and advances its counter by weight.
// weight=1 for normal requests, weight=penaltyMult for off-topic/spam.
// Returns (allowed bool, retryAfter Duration).
func (rl *RateLimiter) Allow(ip string, weight int) (bool, time.Duration) {
	raw, _ := rl.windows.LoadOrStore(ip, &window{})
	w := raw.(*window)
	w.mu.Lock()
	defer w.mu.Unlock()

	now := time.Now()

	// Check active ban.
	if now.Before(w.banUntil) {
		return false, time.Until(w.banUntil)
	}

	cutoff := now.Add(-rl.period)

	// Evict old timestamps.
	filtered := w.ts[:0]
	for _, t := range w.ts {
		if t.After(cutoff) {
			filtered = append(filtered, t)
		}
	}
	w.ts = filtered

	// Check limit before adding.
	if len(w.ts) >= rl.limit {
		retryAt := w.ts[0].Add(rl.period)
		return false, time.Until(retryAt)
	}

	// Add weighted timestamps.
	for i := 0; i < weight; i++ {
		w.ts = append(w.ts, now)
	}

	// If weight > 1 and now over limit → activate ban.
	if len(w.ts) > rl.limit {
		w.banUntil = now.Add(rl.banDuration)
		return false, rl.banDuration
	}

	return true, 0
}

// Ban immediately bans the IP for the configured banDuration.
func (rl *RateLimiter) Ban(ip string) {
	raw, _ := rl.windows.LoadOrStore(ip, &window{})
	w := raw.(*window)
	w.mu.Lock()
	defer w.mu.Unlock()
	w.banUntil = time.Now().Add(rl.banDuration)
}

// cleanupLoop removes stale IP entries every period to prevent memory leaks.
func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(rl.period)
	defer ticker.Stop()
	for range ticker.C {
		cutoff := time.Now().Add(-rl.period)
		rl.windows.Range(func(k, v any) bool {
			w := v.(*window)
			w.mu.Lock()
			stale := len(w.ts) == 0 && time.Now().After(w.banUntil)
			for _, t := range w.ts {
				if t.After(cutoff) {
					stale = false
					break
				}
			}
			w.mu.Unlock()
			if stale {
				rl.windows.Delete(k)
			}
			return true
		})
	}
}
