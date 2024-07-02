//go:build e2e

package cache

import (
	"Soil/cache/internal/errs"
	"context"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestRedisLock_e2e_TryLock(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     "127.0.0.1:6379",
		Password: "",
		DB:       0,
	})
	pong, err := rdb.Ping(context.Background()).Result()
	assert.Equal(t, pong, "PONG")
	require.NoError(t, err)

	testCases := []struct {
		name      string
		before    func(t *testing.T)
		after     func(t *testing.T)
		key       string
		value     string
		wantError error
	}{
		{
			name: "error failed to preempt lock",
			before: func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
				defer cancel()
				ok, er := rdb.SetNX(ctx, "key", "value", 0).Result()
				require.NoError(t, er)
				require.True(t, ok)
			},
			after: func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
				defer cancel()
				delCnt, er := rdb.Del(ctx, "key").Result()
				require.NoError(t, er)
				require.NotEmpty(t, int64(delCnt))
			},
			key:       "key",
			value:     "value",
			wantError: errs.ErrFailedToPreemptLock,
		},
		{
			name: "locked successfully",
			before: func(t *testing.T) {
			},
			after: func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
				defer cancel()
				delCnt, er := rdb.Del(ctx, "key").Result()
				require.NoError(t, er)
				require.NotEmpty(t, int64(delCnt))
			},
			key:   "key",
			value: "value",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.before(t)
			redisLock := NewRedisLock(rdb)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
			defer cancel()
			locked, er := redisLock.TryLock(ctx, tc.key, time.Second*30)
			tc.after(t)
			assert.Equal(t, tc.wantError, er)
			if er != nil {
				return
			}
			assert.Equal(t, locked.key, tc.key)
		})
	}
}

func TestLock_e2e_Unlock(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     "127.0.0.1:6379",
		Password: "",
		DB:       0,
	})
	testCases := []struct {
		name      string
		before    func(t *testing.T)
		after     func(t *testing.T)
		lock      *Lock
		wantError error
	}{
		{
			name:      "key not exist",
			before:    func(t *testing.T) {},
			after:     func(t *testing.T) {},
			wantError: errs.ErrLockNotHold,
			lock: &Lock{
				client: rdb,
				key:    "key",
				uuid:   "value",
			},
		},
		{
			name: "key exist, but this lock belongs to other",
			before: func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
				defer cancel()
				ok, err := rdb.SetNX(ctx, "key", "value", time.Second*10).Result()
				require.NoError(t, err)
				require.True(t, ok)
			},
			after: func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
				defer cancel()
				val, err := rdb.GetDel(ctx, "key").Result()
				require.NoError(t, err)
				require.Equal(t, "value", val)
			},
			wantError: errs.ErrLockNotHold,
			lock: &Lock{
				client: rdb,
				key:    "key",
				uuid:   "123",
			},
		},
		{
			name: "unlock success",
			before: func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
				defer cancel()
				ok, err := rdb.SetNX(ctx, "key", "value", time.Second*10).Result()
				require.NoError(t, err)
				require.True(t, ok)
			},
			after: func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
				defer cancel()
				_, err := rdb.GetDel(ctx, "key").Result()
				assert.Equal(t, err.Error(), "redis: nil")
			},
			lock: &Lock{
				client: rdb,
				key:    "key",
				uuid:   "value",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.before(t)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
			defer cancel()
			err := tc.lock.Unlock(ctx)
			tc.after(t)
			assert.Equal(t, tc.wantError, err)
			if err != nil {
				return
			}
		})
	}
}

func TestLock_e2e_Refresh(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     "127.0.0.1:6379",
		Password: "",
		DB:       0,
	})
	testCases := []struct {
		name      string
		before    func(t *testing.T)
		after     func(t *testing.T)
		lock      *Lock
		wantError error
	}{
		{
			name:      "key not exist",
			before:    func(t *testing.T) {},
			after:     func(t *testing.T) {},
			wantError: errs.ErrLockNotHold,
			lock: &Lock{
				client:     rdb,
				key:        "key",
				uuid:       "value",
				expiration: time.Second * 10,
			},
		},
		{
			name: "key exist, but this lock belongs to other",
			before: func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
				defer cancel()
				ok, err := rdb.SetNX(ctx, "key", "value", time.Second*10).Result()
				require.NoError(t, err)
				require.True(t, ok)
			},
			after: func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
				defer cancel()
				ttl, err := rdb.TTL(ctx, "key").Result()
				require.NoError(t, err)
				// 在重置过期时间为1min后，分布式锁的ttl还是比在before函数中设置的timeout(10s)少表示重置失败
				require.True(t, ttl <= time.Second*10)
				val, err := rdb.GetDel(ctx, "key").Result()
				require.NoError(t, err)
				require.Equal(t, "value", val)
			},
			wantError: errs.ErrLockNotHold,
			lock: &Lock{
				client:     rdb,
				key:        "key",
				uuid:       "123",
				expiration: time.Minute,
			},
		},
		{
			name: "refresh success",
			before: func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
				defer cancel()
				ok, err := rdb.SetNX(ctx, "key", "value", time.Second*10).Result()
				require.NoError(t, err)
				require.True(t, ok)
			},
			after: func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
				defer cancel()
				ttl, err := rdb.TTL(ctx, "key").Result()
				require.NoError(t, err)
				require.True(t, ttl > time.Second*10)
				val, err := rdb.GetDel(ctx, "key").Result()
				require.NoError(t, err)
				require.Equal(t, "value", val)
			},
			lock: &Lock{
				client:     rdb,
				key:        "key",
				uuid:       "value",
				expiration: time.Minute,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.before(t)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
			defer cancel()
			err := tc.lock.Refresh(ctx)
			tc.after(t)
			assert.Equal(t, tc.wantError, err)
			if err != nil {
				return
			}

		})
	}
}
