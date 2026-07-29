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
	// Allow records one hit against key and reports whether it stays within
	// limit requests per window.
	//
	// Result is only meaningful when err is nil — on error it is the zero
	// value, whose Allowed is false. Callers MUST branch on err before reading
	// Allowed, or a backend outage reads as "deny everything". Whether an
	// outage fails open or closed is deliberately the caller's policy: the HTTP
	// middleware fails open, so an infra blip degrades protection rather than
	// taking the API down.
	Allow(ctx context.Context, key string, limit int, window time.Duration) (Result, error)
}
