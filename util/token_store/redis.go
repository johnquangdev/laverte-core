//nolint:staticcheck // package name required by API contract (Tasks 5 and 6 import token_store.ITokenStore)
package token_store

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/johnquangdev/laverte-home/config"
)

type redisStore struct{ client *redis.Client }

func NewRedis(cfg *config.Config) ITokenStore {
	opt, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		panic("util/token_store/redis: " + err.Error())
	}
	return &redisStore{client: redis.NewClient(opt)}
}

func (r *redisStore) SaveState(ctx context.Context, state string) error {
	return r.client.Set(ctx, "oauth:state:"+state, "1", 10*time.Minute).Err()
}

func (r *redisStore) ValidateState(ctx context.Context, state string) (bool, error) {
	n, err := r.client.Del(ctx, "oauth:state:"+state).Result()
	return n > 0, err
}

func (r *redisStore) BlacklistToken(ctx context.Context, tokenID string) error {
	return r.client.Set(ctx, "jwt:blacklist:"+tokenID, "1", 24*time.Hour).Err()
}

func (r *redisStore) IsBlacklisted(ctx context.Context, tokenID string) (bool, error) {
	n, err := r.client.Exists(ctx, "jwt:blacklist:"+tokenID).Result()
	return n > 0, err
}
