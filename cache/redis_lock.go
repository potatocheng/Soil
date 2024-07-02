package cache

import (
	"Soil/cache/internal/errs"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"sync"
	"time"
)

var (
	//go:embed lua/unlock.lua
	luaUnlock string
	//go:embed lua/refresh.lua
	luaRefresh string
	//go:embed lua/lock.lua
	luaLock string
)

type RedisLock struct {
	client redis.Cmdable
	sync.Mutex
}

func NewRedisLock(client redis.Cmdable) *RedisLock {
	return &RedisLock{
		client: client,
	}
}

func (r *RedisLock) TryLock(ctx context.Context, key string, timeout time.Duration) (*Lock, error) {
	uuidStr := uuid.NewString()
	ok, err := r.client.SetNX(ctx, key, uuidStr, timeout).Result()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errs.ErrFailedToPreemptLock
	}

	return &Lock{
		client:       r.client,
		key:          key,
		uuid:         uuidStr,
		expiration:   timeout,
		unlockedChan: make(chan struct{}, 1),
	}, nil
}

type Lock struct {
	client       redis.Cmdable
	key          string
	uuid         string
	expiration   time.Duration
	unlockedChan chan struct{}
}

func (l *Lock) Unlock(ctx context.Context) error {
	//value, err := l.client.Get(ctx, l.key).Result()
	//if err != nil {
	//	return err
	//}
	//if value != l.uuid {
	//	return errs.ErrLockNotHold
	//}
	// 如果在检查完成后，有键值对到期被删掉，又有一个实例加了锁，这里就会解除掉不是该实例加的锁
	//n, err := l.client.Del(ctx, l.key).Result()
	//if err != nil {
	//	return err
	//}
	//if n != 1 {
	//	return errs.ErrLockNotHold
	//}
	//return nil
	res, err := l.client.Eval(ctx, luaUnlock, []string{l.key}, l.uuid).Int64()
	defer func() {
		close(l.unlockedChan)
	}()
	if err != nil {
		return err
	}
	if res != 1 {
		//解锁失败
		return errs.ErrLockNotHold
	}
	return nil
}

func (l *Lock) Refresh(ctx context.Context) error {
	//value, err := l.client.Get(ctx, l.key).Result()
	//if err != nil {
	//	return err
	//}
	//if value != l.uuid {
	//	return errs.ErrLockNotHold
	//}
	// 如果在检查完成后，有键值对到期被删掉，又有一个实例加了锁，这里就会解除掉不是该实例加的锁
	//n, err := l.client.Del(ctx, l.key).Result()
	//if err != nil {
	//	return err
	//}
	//if n != 1 {
	//	return errs.ErrLockNotHold
	//}
	//return nil
	res, err := l.client.Eval(ctx, luaRefresh, []string{l.key}, l.uuid, l.expiration.Seconds()).Int64()
	if err != nil {
		return err
	}
	if res != 1 {
		//更新过期时间失败
		return errs.ErrLockNotHold
	}
	return nil
}

// AutoRefresh interval参数：在interval间隔内业务未完成进行续约
func (l *Lock) AutoRefresh(ctx context.Context, interval time.Duration,
	timeout time.Duration) error {
	timeoutChan := make(chan struct{})
	for {
		ticker := time.NewTicker(interval)
		select {
		case <-ticker.C:
			timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
			err := l.Refresh(timeoutCtx)
			cancel()
			if err != nil {
				return err
			}
			if errors.Is(err, context.DeadlineExceeded) {
				// 续约失败，重试
				timeoutChan <- struct{}{}
				continue
			}
		case <-timeoutChan:
			// 续约失败，重试
			timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
			err := l.Refresh(timeoutCtx)
			cancel()
			if err != nil {
				return err
			}
			if errors.Is(err, context.DeadlineExceeded) {
				// 续约失败，重试
				timeoutChan <- struct{}{}
				continue
			}
		case <-l.unlockedChan:
			return nil
		}
	}
}

// Lock 如果加锁失败，重试
// expiration表示redis锁(redis键值对)的过期时间
// timeout表示context设置的过期时间
func (r *RedisLock) Lock(ctx context.Context, key string,
	expiration time.Duration,
	timeout time.Duration,
	strategy RetryStrategy) (*Lock, error) {

	// 加锁失败，重试
	var timer *time.Timer
	val := uuid.NewString()
	for {
		timeoutCtx, cancel := context.WithTimeout(ctx, timeout)

		res, err := r.client.Eval(timeoutCtx, luaLock, []string{key}, val, expiration).Result()
		cancel()
		if err != nil && !errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		if res.(string) == "OK" {
			// 加锁成功
			return &Lock{
				client:       r.client,
				key:          key,
				uuid:         val,
				expiration:   expiration,
				unlockedChan: make(chan struct{}, 1),
			}, nil
		}

		interval, ok := strategy.Next()
		if !ok {
			return nil, fmt.Errorf("redis-lock: 超出重试次数, %w", errs.ErrFailedToPreemptLock)
		}
		if nil == timer {
			timer = time.NewTimer(interval)
		} else {
			timer.Reset(interval)
		}

		select {
		case <-timer.C:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

}
