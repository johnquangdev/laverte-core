package ratelimit

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/johnquangdev/laverte-home/config"
)

type redisLimiter struct{ client *redis.Client }

func NewRedis(cfg *config.Config) ILimiter {
	opt, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		panic("util/ratelimit/redis: " + err.Error())
	}
	return &redisLimiter{client: redis.NewClient(opt)}
}

func (r *redisLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (Result, error) {
	pipe := r.client.Pipeline()
	incr := pipe.Incr(ctx, key)
	pipe.ExpireNX(ctx, key, window)
	pttl := pipe.PTTL(ctx, key)
	if _, err := pipe.Exec(ctx); err != nil {
		return Result{}, err
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
