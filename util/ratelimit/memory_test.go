package ratelimit

import (
	"context"
	"testing"
	"time"
)

func TestMemoryLimiterAllowsUpToLimit(t *testing.T) {
	l := NewMemory()
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		res, err := l.Allow(ctx, "k1", 3, time.Minute)
		if err != nil {
			t.Fatalf("Allow() error = %v", err)
		}
		if !res.Allowed {
			t.Fatalf("Allow() call %d = denied, want allowed", i+1)
		}
	}

	res, err := l.Allow(ctx, "k1", 3, time.Minute)
	if err != nil {
		t.Fatalf("Allow() error = %v", err)
	}
	if res.Allowed {
		t.Fatal("Allow() call 4 = allowed, want denied")
	}
	if res.RetryAfter <= 0 {
		t.Error("RetryAfter should be positive when denied")
	}
}

func TestMemoryLimiterKeysAreIndependent(t *testing.T) {
	l := NewMemory()
	ctx := context.Background()

	res, err := l.Allow(ctx, "k2", 1, time.Minute)
	if err != nil || !res.Allowed {
		t.Fatalf("Allow(k2) = %+v, %v", res, err)
	}
	res, err = l.Allow(ctx, "k3", 1, time.Minute)
	if err != nil || !res.Allowed {
		t.Fatalf("Allow(k3) = %+v, %v", res, err)
	}
}
