package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/johnquangdev/laverte-core/config"
)

type redisLimiter struct{ client *redis.Client }

func NewRedis(cfg *config.Config) ILimiter {
	// ParseURL's error embeds the URL, which can carry a password — keep it out
	// of a panic that lands in crash logs.
	opt, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		panic("util/ratelimit/redis: REDIS_URL is not a valid redis:// URL")
	}
	return &redisLimiter{client: redis.NewClient(opt)}
}

// Allow uses a plain pipeline on purpose — no MULTI/EXEC, no Lua script. INCR
// alone is atomic, so concurrent callers on one key each get a distinct
// strictly-increasing count, and that count alone decides the verdict. ExpireNX
// is idempotent, so racing first-hits settle on whichever TTL lands first, and
// PTTL only feeds the Retry-After hint.
func (r *redisLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (Result, error) {
	pipe := r.client.Pipeline()
	incr := pipe.Incr(ctx, key)
	pipe.ExpireNX(ctx, key, window)
	pttl := pipe.PTTL(ctx, key)
	if _, err := pipe.Exec(ctx); err != nil {
		return Result{}, fmt.Errorf("ratelimit: allow %q: %w", key, err)
	}

	if incr.Val() <= int64(limit) {
		return Result{Allowed: true}, nil
	}

	retryAfter := pttl.Val()
	if retryAfter <= 0 {
		retryAfter = window
	}
	return Result{Allowed: false, RetryAfter: retryAfter}, nil
}
