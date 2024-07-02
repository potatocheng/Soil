package cache

import (
	"Soil/cache/internal/errs"
	"context"
	"errors"
	"fmt"
	"log"
	"time"
)

// ReadThroughCache 所谓ReadThrough就是用户只与缓存模块交互，不在管与数据库交互
// 更新数据库的操作由缓存自己代理
type ReadThroughCache struct {
	Cache
	LoadFunc   func(ctx context.Context, key string) (any, error) // 用户自行编写数据库取数据逻辑
	expiration time.Duration
}

// Get 同步Get
func (r *ReadThroughCache) Get(ctx context.Context, key string) (any, error) {
	value, err := r.Get(ctx, key)
	if errors.Is(err, errs.NewErrKeyNotFound(key)) {
		// 在缓存中没有找到数据，去数据库中取数据
		value, err = r.LoadFunc(ctx, key)
		if err == nil {
			// 数据库有数据，更新缓存
			err = r.Cache.Set(ctx, key, value, r.expiration)
			if err != nil {
				return value, fmt.Errorf("%w, 原因：%s", errs.ErrFailedToRefreshCache, err.Error())
			}
		}
	}
	return value, err
}

// AsyncGet Cache直接返回响应，而后异步从DB读取数据刷新缓存
func (r *ReadThroughCache) AsyncGet(ctx context.Context, key string) (any, error) {
	value, err := r.Get(ctx, key)
	if errors.Is(err, errs.NewErrKeyNotFound(key)) {
		go func() {
			// 在缓存中没有找到数据，去数据库中取数据
			value, err = r.LoadFunc(ctx, key)
			if err == nil {
				// 数据库有数据，更新缓存
				err = r.Cache.Set(ctx, key, value, r.expiration)
				if err != nil {
					log.Fatalln(value, fmt.Errorf("%w, 原因：%s", errs.ErrFailedToRefreshCache, err.Error()))
				}
			}
		}()
	}
	return value, err
}

// SemiAsyncGet Cache从缓存读取数据是同步的，但是将返回值是异步刷新到缓存的
func (r *ReadThroughCache) SemiAsyncGet(ctx context.Context, key string) (any, error) {
	value, err := r.Get(ctx, key)
	if errors.Is(err, errs.NewErrKeyNotFound(key)) {
		// 在缓存中没有找到数据，去数据库中取数据
		value, err = r.LoadFunc(ctx, key)
		if err == nil {
			go func() {
				// 数据库有数据，更新缓存
				err = r.Cache.Set(ctx, key, value, r.expiration)
				if err != nil {
					log.Fatalln(value, fmt.Errorf("%w, 原因：%s", errs.ErrFailedToRefreshCache, err.Error()))
				}
			}()
		}
	}
	return value, err
}
