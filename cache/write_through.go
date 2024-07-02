package cache

import (
	"context"
	"log"
	"time"
)

// WriteThroughCache 缓存模式，开发者只需要写入cache，cache自己会更新数据库
type WriteThroughCache struct {
	Cache
	storeFunc func(ctx context.Context, key string, val any, expiration time.Duration) error
}

func (w *WriteThroughCache) Set(ctx context.Context, key string, val any, expiration time.Duration) error {
	err := w.storeFunc(ctx, key, val, expiration)
	if err != nil {
		return err
	}
	return w.Cache.Set(ctx, key, val, expiration)
}

// SemiAsyncSet 一般使用半异步的方法
func (w *WriteThroughCache) SemiAsyncSet(ctx context.Context, key string, val any, expiration time.Duration) error {
	err := w.storeFunc(ctx, key, val, expiration)
	go func() {
		err = w.Cache.Set(ctx, key, val, expiration)
		if err != nil {
			log.Fatalln(err)
		}
	}()
	return err
}

// AsyncSet 几乎不会用，因为开发者得不到反馈
func (w *WriteThroughCache) AsyncSet(ctx context.Context, key string, val any, expiration time.Duration) error {
	go func() {
		err := w.storeFunc(ctx, key, val, expiration)
		if err != nil {
			log.Fatalln(err)
		}
		err = w.Cache.Set(ctx, key, val, expiration)
		if err != nil {
			log.Fatalln(err)
		}
	}()
	return nil
}
