package tokenstore

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/johnquangdev/laverte-home/config"
)

type redisStore struct {
	client       *redis.Client
	blacklistTTL time.Duration
}

func NewRedis(cfg *config.Config) ITokenStore {
	// ParseURL's error embeds the URL, which can carry a password — keep it out
	// of a panic that lands in crash logs.
	opt, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		panic("util/tokenstore/redis: REDIS_URL is not a valid redis:// URL")
	}
	// A blacklist entry only has to outlive the token it revokes. Derived from
	// the access TTL rather than hardcoded, so raising JWT_ACCESS_TTL_MINUTES
	// can't silently leave revoked tokens usable again once the key expires.
	// The extra minute absorbs clock skew between this process and Redis.
	ttl := time.Duration(cfg.JWTAccessTTLMinutes)*time.Minute + time.Minute
	return &redisStore{client: redis.NewClient(opt), blacklistTTL: ttl}
}

func (r *redisStore) SaveState(ctx context.Context, state string) error {
	return r.client.Set(ctx, "oauth:state:"+state, "1", 10*time.Minute).Err()
}

func (r *redisStore) ValidateState(ctx context.Context, state string) (bool, error) {
	n, err := r.client.Del(ctx, "oauth:state:"+state).Result()
	return n > 0, err
}

func (r *redisStore) BlacklistToken(ctx context.Context, tokenID string) error {
	return r.client.Set(ctx, "jwt:blacklist:"+tokenID, "1", r.blacklistTTL).Err()
}

func (r *redisStore) IsBlacklisted(ctx context.Context, tokenID string) (bool, error) {
	n, err := r.client.Exists(ctx, "jwt:blacklist:"+tokenID).Result()
	return n > 0, err
}
