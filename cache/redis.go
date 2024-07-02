package cache

import (
	"Soil/cache/internal/errs"
	"context"
	"github.com/redis/go-redis/v9"
	"time"
)

type RedisCache struct {
	client redis.Cmdable
}

func NewRedisCache(client redis.Cmdable) *RedisCache {
	return &RedisCache{client}
}

func (r RedisCache) Set(ctx context.Context, key string, val any, expiration time.Duration) error {
	res, err := r.client.Set(ctx, key, val, expiration).Result()
	if err != nil {
		return err
	}
	if res != "OK" {
		return errs.NewErrRedisSetFailed(res)
	}

	return nil
}

func (r RedisCache) Get(ctx context.Context, key string) (any, error) {
	return r.client.Get(ctx, key).Result()
}

func (r RedisCache) Delete(ctx context.Context, key string) error {
	_, err := r.client.Del(ctx, key).Result()
	return err
}
