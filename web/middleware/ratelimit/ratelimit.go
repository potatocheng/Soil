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

	// ttl 为按 IP 限流时单个桶的过期时间。距上次访问超过 ttl 的桶会在
	// 下次该 IP 请求到达时被懒清理（删除并重建为满桶）。
	// 0 表示永不过期（向后兼容）。默认 10 秒。
	ttl time.Duration

	// 全局限流时持有的单个桶
	bucket *tokenBucket

	// 按 IP 限流时持有的桶集合。配合 ttl 实现懒清理：
	// 请求到达时检查目标桶是否过期，过期则删除并新建满桶。
	// 内存占用上限约为 rate × ttl 个桶。
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
		ttl:      10 * time.Second,
	}
}

// WithByIP 设置是否按客户端 IP 维度限流。支持链式调用。
func (mb *MiddlewareBuilder) WithByIP(b bool) *MiddlewareBuilder {
	mb.byIP = b
	return mb
}

// WithTTL 设置按 IP 限流时单个桶的过期时间。支持链式调用。
// 距上次访问超过 ttl 的桶会在下次该 IP 请求到达时被清理。
// 传 0 表示永不过期（关闭懒清理，仅在内存充裕且 IP 数量可控时使用）。
func (mb *MiddlewareBuilder) WithTTL(ttl time.Duration) *MiddlewareBuilder {
	mb.ttl = ttl
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
				bucket = mb.getOrCreateBucket(ip)
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

// getOrCreateBucket 获取或创建指定 IP 的令牌桶。
// 若桶已过期（距上次访问超过 ttl），则删除旧桶并新建满桶。
// 采用懒清理策略：仅在请求到达时检查过期，无后台 goroutine。
func (mb *MiddlewareBuilder) getOrCreateBucket(ip string) *tokenBucket {
	// 先读锁快查
	mb.bucketsMu.RLock()
	bucket := mb.buckets[ip]
	mb.bucketsMu.RUnlock()

	// 检查是否过期，过期则删除
	if bucket != nil && mb.isBucketExpired(bucket) {
		mb.bucketsMu.Lock()
		// Double-check：可能已被其他 goroutine 重建或刷新
		if current, ok := mb.buckets[ip]; ok && current == bucket {
			delete(mb.buckets, ip)
			bucket = nil
		}
		mb.bucketsMu.Unlock()
	}

	// 未命中或刚被清理：创建新桶
	if bucket == nil {
		bucket = newTokenBucket(mb.rate, mb.capacity)
		mb.bucketsMu.Lock()
		if existing, ok := mb.buckets[ip]; ok {
			bucket = existing
		} else {
			mb.buckets[ip] = bucket
		}
		mb.bucketsMu.Unlock()
	}

	return bucket
}

// isBucketExpired 检查桶是否已过期（距上次访问超过 ttl）。
// 通过读取 bucket.lastTime 判断（take() 每次都会更新 lastTime）。
// ttl <= 0 时永不过期。
func (mb *MiddlewareBuilder) isBucketExpired(bucket *tokenBucket) bool {
	if mb.ttl <= 0 {
		return false
	}
	bucket.mu.Lock()
	defer bucket.mu.Unlock()
	return time.Since(bucket.lastTime) > mb.ttl
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
