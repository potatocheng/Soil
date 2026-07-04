// Package ratelimit 提供基于自实现令牌桶算法的限流中间件。
//
// 不依赖 golang.org/x/time/rate，仅使用 sync.Mutex + 时间戳实现。
package ratelimit

import (
	"Soil/web"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// tokenBucket 自实现的令牌桶。
// capacity 为桶容量（burst），rate 为每秒令牌填充速率，
// tokens 为当前令牌数（float 以支持小数累积），lastTime 为上次填充时间。
type tokenBucket struct {
	capacity float64
	rate     float64
	tokens   float64
	lastTime time.Time
	mu       sync.Mutex
}

func newTokenBucket(rate float64, capacity int64) *tokenBucket {
	return &tokenBucket{
		capacity: float64(capacity),
		rate:     rate,
		// 初始时桶满，允许瞬时 burst。
		tokens:   float64(capacity),
		lastTime: time.Now(),
	}
}

// allow 尝试消费 1 个令牌，返回是否成功。
// 根据时间差填充令牌（不超过 capacity），再消费 1 个令牌。
func (b *tokenBucket) allow() bool {
	allowed, _ := b.take()
	return allowed
}

// take 尝试消费 1 个令牌，返回是否成功以及消费后剩余的令牌数。
// 在同一把锁内完成填充与消费，保证返回的剩余数与本次决策一致。
func (b *tokenBucket) take() (bool, float64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(b.lastTime).Seconds()
	if elapsed > 0 {
		b.tokens += elapsed * b.rate
		if b.tokens > b.capacity {
			b.tokens = b.capacity
		}
		b.lastTime = now
	}

	if b.tokens >= 1 {
		b.tokens--
		return true, b.tokens
	}
	return false, b.tokens
}

// MiddlewareBuilder 限流中间件构建器。
type MiddlewareBuilder struct {
	rate     float64 // 每秒令牌填充速率（RPS）
	capacity int64   // 桶容量（burst）
	byIP     bool    // 是否按客户端 IP 维度限流；false 为全局限流

	// 全局限流时持有的单个桶
	bucket *tokenBucket

	// 按 IP 限流时持有的桶集合
	// 已知限制：该 map 不做 LRU 淘汰，长期运行下不同 IP 数量无上限，
	// 可能导致内存持续增长。生产环境如需更严格的内存控制，
	// 应引入过期淘汰策略（如 LRU/TTL）。
	buckets   map[string]*tokenBucket
	bucketsMu sync.RWMutex
}

// Create 创建限流中间件构建器。
//   - rate: 每秒令牌填充速率（RPS）
//   - capacity: 桶容量（burst），即瞬时允许的最大请求数
func Create(rate float64, capacity int64) *MiddlewareBuilder {
	return &MiddlewareBuilder{
		rate:     rate,
		capacity: capacity,
	}
}

// WithByIP 设置是否按客户端 IP 维度限流。支持链式调用。
func (mb *MiddlewareBuilder) WithByIP(b bool) *MiddlewareBuilder {
	mb.byIP = b
	return mb
}

// Build 构造限流中间件。在 Build 时初始化对应的桶结构。
func (mb *MiddlewareBuilder) Build() web.Middleware {
	if mb.byIP {
		mb.buckets = make(map[string]*tokenBucket)
	} else {
		mb.bucket = newTokenBucket(mb.rate, mb.capacity)
	}

	return func(next web.HandleFunc) web.HandleFunc {
		return func(ctx *web.Context) {
			var bucket *tokenBucket
			if mb.byIP {
				ip := clientIP(ctx.Req)
				// 先读锁快查，命中则直接复用
				mb.bucketsMu.RLock()
				bucket = mb.buckets[ip]
				mb.bucketsMu.RUnlock()
				if bucket == nil {
					// 未命中则加写锁创建，double-check 防止重复创建
					bucket = newTokenBucket(mb.rate, mb.capacity)
					mb.bucketsMu.Lock()
					if existing, ok := mb.buckets[ip]; ok {
						bucket = existing
					} else {
						mb.buckets[ip] = bucket
					}
					mb.bucketsMu.Unlock()
				}
			} else {
				bucket = mb.bucket
			}

			allowed, remaining := bucket.take()
			if allowed {
				ctx.SetHeader("X-RateLimit-Remaining", strconv.Itoa(int(remaining)))
				next(ctx)
				return
			}

			// 拒绝：ctx.Abort 会将 done 置为 true，
			// server.flashResp 因此不再回写状态码/响应体/响应头，
			// 所以这里直接写入 ResponseWriter 以确保 429 与
			// X-RateLimit-Remaining: 0 真正到达客户端。
			ctx.Resp.Header().Set("X-RateLimit-Remaining", "0")
			ctx.Resp.WriteHeader(http.StatusTooManyRequests)
			_, _ = ctx.Resp.Write([]byte("Too Many Requests"))
			ctx.Abort(http.StatusTooManyRequests, "Too Many Requests")
		}
	}
}

// clientIP 获取客户端 IP。
// 优先取 X-Forwarded-For 的第一个 IP，回退 X-Real-IP，再回退 RemoteAddr（去掉端口）。
func clientIP(req *http.Request) string {
	if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
		// X-Forwarded-For 可能形如 "client, proxy1, proxy2"，取第一个
		if idx := strings.IndexByte(xff, ','); idx >= 0 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}
	if xri := req.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	// RemoteAddr 形如 "host:port"，去掉端口
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		return strings.TrimSpace(req.RemoteAddr)
	}
	return host
}
