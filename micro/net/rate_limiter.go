package net

import "context"

// RateLimiter 是服务端速率限制接入点
type RateLimiter interface {
	// Allow 返回是否允许当前请求通过
	Allow(ctx context.Context) bool
}

// unlimitedLimiter 默认不限流
type unlimitedLimiter struct{}

func (u unlimitedLimiter) Allow(_ context.Context) bool { return true }

var defaultRateLimiter RateLimiter = unlimitedLimiter{}
