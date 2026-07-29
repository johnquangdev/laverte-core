package ratelimit

import (
	"context"
	"time"
)

type Result struct {
	Allowed    bool
	RetryAfter time.Duration
}

// ILimiter is a fixed-window request counter keyed by an arbitrary string
// (per-IP or per-phone). Backed by Redis in production, in-memory for tests.
type ILimiter interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (Result, error)
}
