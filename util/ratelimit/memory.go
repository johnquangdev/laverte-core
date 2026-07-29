package ratelimit

import (
	"context"
	"sync"
	"time"
)

type memoryLimiter struct {
	mu     sync.Mutex
	counts map[string]*window
}

type window struct {
	count   int
	expires time.Time
}

func NewMemory() ILimiter {
	return &memoryLimiter{counts: make(map[string]*window)}
}

func (m *memoryLimiter) Allow(_ context.Context, key string, limit int, win time.Duration) (Result, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	w, ok := m.counts[key]
	if !ok || now.After(w.expires) {
		w = &window{count: 0, expires: now.Add(win)}
		m.counts[key] = w
	}
	w.count++

	if w.count <= limit {
		return Result{Allowed: true}, nil
	}
	return Result{Allowed: false, RetryAfter: w.expires.Sub(now)}, nil
}
