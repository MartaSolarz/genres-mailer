package auth

import (
	"sync"
	"time"
)

type RateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	max      int
	window   time.Duration
}

func NewRateLimiter(maxAttempts int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		attempts: make(map[string][]time.Time),
		max:      maxAttempts,
		window:   window,
	}
}

func (r *RateLimiter) Allowed(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	return len(r.prune(key)) < r.max
}

func (r *RateLimiter) RecordFailure(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.attempts[key] = append(r.prune(key), time.Now())
}

func (r *RateLimiter) Reset(key string) {
	r.mu.Lock()
	delete(r.attempts, key)
	r.mu.Unlock()
}

func (r *RateLimiter) prune(key string) []time.Time {
	cutoff := time.Now().Add(-r.window)

	kept := r.attempts[key][:0]
	for _, t := range r.attempts[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}

	r.attempts[key] = kept

	return kept
}
