package ratelimit

import (
	"Soil/web"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// newServerWith 构建一个挂载了限流中间件、路由 /test 返回 200 的服务器。
func newServerWith(builder *MiddlewareBuilder) *web.HTTPServer {
	httpServer := web.NewHttpServer()
	httpServer.Use(builder.Build())
	httpServer.Get("/test", func(ctx *web.Context) {
		ctx.RespStatusCode = http.StatusOK
		ctx.RespData = []byte("OK")
	})
	return httpServer
}

// TR-7.1: rate=10, capacity=10，瞬间发送 100 个并发请求，
// 统计返回 429 的数量 > 0，返回 200 的数量 <= capacity。
func TestRatelimit_Concurrent(t *testing.T) {
	httpServer := newServerWith(Create(10, 10))

	const total = 100
	var status200, status429 int64

	var wg sync.WaitGroup
	// start 充当起跑栅栏，尽量让 100 个 goroutine 同时发起请求。
	start := make(chan struct{})

	for i := 0; i < total; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// 每个 goroutine 使用自己的 Request 与 ResponseRecorder
			// （httptest.ResponseRecorder 非并发安全）。
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			w := httptest.NewRecorder()
			<-start
			httpServer.ServeHTTP(w, req)
			switch w.Code {
			case http.StatusOK:
				atomic.AddInt64(&status200, 1)
			case http.StatusTooManyRequests:
				atomic.AddInt64(&status429, 1)
			}
		}()
	}
	close(start)
	wg.Wait()

	t.Logf("concurrent: total=%d, status200=%d, status429=%d", total, status200, status429)
	assert.True(t, status429 > 0, "应存在被限流（429）的请求")
	assert.True(t, status200 <= 10, "成功请求数不应超过桶容量 capacity=10")
}

// TR-7.2: 429 响应头含 X-RateLimit-Remaining 且值为 "0"。
func TestRatelimit_TooManyRequestsHeader(t *testing.T) {
	// 使用 rate=1, capacity=1，便于用顺序请求触发 429。
	httpServer := newServerWith(Create(1, 1))

	// 第一个请求消耗唯一令牌，返回 200。
	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	w1 := httptest.NewRecorder()
	httpServer.ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusOK, w1.Code)
	assert.Equal(t, "0", w1.Header().Get("X-RateLimit-Remaining"))

	// 紧接着的第二个请求因令牌不足被拒绝，返回 429 且剩余为 0。
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	w2 := httptest.NewRecorder()
	httpServer.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusTooManyRequests, w2.Code)
	assert.Equal(t, "Too Many Requests", w2.Body.String())
	assert.Equal(t, "0", w2.Header().Get("X-RateLimit-Remaining"))
}

// TR-7.4: 按 IP 限流时，不同 RemoteAddr 的请求各自独立计数。
func TestRatelimit_ByIP(t *testing.T) {
	// rate=1, capacity=1，每个 IP 独立拥有 1 个令牌。
	httpServer := newServerWith(Create(1, 1).WithByIP(true))

	// IP1 第一次请求成功。
	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req1.RemoteAddr = "1.1.1.1:1234"
	w1 := httptest.NewRecorder()
	httpServer.ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusOK, w1.Code)

	// IP1 第二次请求被限流。
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.RemoteAddr = "1.1.1.1:1234"
	w2 := httptest.NewRecorder()
	httpServer.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusTooManyRequests, w2.Code)

	// IP2 拥有独立桶，仍可成功请求。
	req3 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req3.RemoteAddr = "2.2.2.2:5678"
	w3 := httptest.NewRecorder()
	httpServer.ServeHTTP(w3, req3)
	assert.Equal(t, http.StatusOK, w3.Code)
}

// TR-7.3: 限流器在 100 并发下无 panic。
func TestRatelimit_NoPanicUnderConcurrency(t *testing.T) {
	httpServer := newServerWith(Create(10, 10))

	const total = 100
	var panicked int64

	var wg sync.WaitGroup
	start := make(chan struct{})

	for i := 0; i < total; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					atomic.AddInt64(&panicked, 1)
				}
			}()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			w := httptest.NewRecorder()
			<-start
			httpServer.ServeHTTP(w, req)
		}()
	}
	close(start)
	wg.Wait()

	assert.Equal(t, int64(0), panicked, "并发下不应发生 panic")
}

// TestRatelimit_TTL_Expiration 验证桶过期后会被清理，该 IP 下次请求获得全新满桶。
// rate=1, capacity=1, ttl=80ms：
//   - 第一次请求消耗唯一令牌 → 200
//   - ttl 内立即再请求 → 429（桶未过期，令牌仍为 0）
//   - 等待 ttl 过期后再请求 → 200（旧桶被清理，新桶满载）
func TestRatelimit_TTL_Expiration(t *testing.T) {
	builder := Create(1, 1).WithByIP(true).WithTTL(80 * time.Millisecond)
	httpServer := newServerWith(builder)

	ip := "9.9.9.9:9999"

	// 第一次请求：消耗唯一令牌
	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req1.RemoteAddr = ip
	w1 := httptest.NewRecorder()
	httpServer.ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusOK, w1.Code)

	// 桶未过期，立即再请求 → 429
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.RemoteAddr = ip
	w2 := httptest.NewRecorder()
	httpServer.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusTooManyRequests, w2.Code)

	// 桶存在
	builder.bucketsMu.RLock()
	assert.Len(t, builder.buckets, 1)
	builder.bucketsMu.RUnlock()

	// 等待 TTL 过期
	time.Sleep(120 * time.Millisecond)

	// 过期后请求：旧桶被懒清理，新建满桶 → 200
	req3 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req3.RemoteAddr = ip
	w3 := httptest.NewRecorder()
	httpServer.ServeHTTP(w3, req3)
	assert.Equal(t, http.StatusOK, w3.Code, "TTL 过期后应获得新满桶")

	// 新桶仍在 map 中（刚被访问）
	builder.bucketsMu.RLock()
	assert.Len(t, builder.buckets, 1)
	builder.bucketsMu.RUnlock()
}

// TestRatelimit_TTL_WithinWindow 验证 TTL 内连续访问不会触发清理。
func TestRatelimit_TTL_WithinWindow(t *testing.T) {
	builder := Create(5, 5).WithByIP(true).WithTTL(200 * time.Millisecond)
	httpServer := newServerWith(builder)

	remoteAddr := "8.8.8.8:8888"
	// clientIP 会去掉端口，map key 是 "8.8.8.8"
	ip := clientIP(&http.Request{RemoteAddr: remoteAddr})

	// 在 TTL 窗口内连续请求 3 次
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = remoteAddr
		w := httptest.NewRecorder()
		httpServer.ServeHTTP(w, req)
		time.Sleep(50 * time.Millisecond) // < ttl(200ms)
	}

	// 桶应仍存在
	builder.bucketsMu.RLock()
	bucket := builder.buckets[ip]
	builder.bucketsMu.RUnlock()
	assert.NotNil(t, bucket, "TTL 内连续访问不应清理桶")
}

// TestRatelimit_TTL_Zero_DisablesExpiration 验证 WithTTL(0) 关闭过期清理。
func TestRatelimit_TTL_Zero_DisablesExpiration(t *testing.T) {
	builder := Create(1, 1).WithByIP(true).WithTTL(0)
	httpServer := newServerWith(builder)

	remoteAddr := "7.7.7.7:7777"
	ip := clientIP(&http.Request{RemoteAddr: remoteAddr})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = remoteAddr
	w := httptest.NewRecorder()
	httpServer.ServeHTTP(w, req)

	// 等待一段较长时间（远超默认 10s ttl，但 ttl=0 不过期）
	// 这里用 150ms 模拟，足以验证 isBucketExpired 返回 false
	time.Sleep(150 * time.Millisecond)

	// 桶应未被清理
	builder.bucketsMu.RLock()
	assert.Len(t, builder.buckets, 1)
	bucket := builder.buckets[ip]
	builder.bucketsMu.RUnlock()
	assert.NotNil(t, bucket, "ttl=0 时桶不应被清理")

	// isBucketExpired 应返回 false
	assert.False(t, builder.isBucketExpired(bucket))
}

// TestRatelimit_TTL_DefaultIsTenSeconds 验证 Create() 默认 ttl=10s。
func TestRatelimit_TTL_DefaultIsTenSeconds(t *testing.T) {
	builder := Create(10, 10)
	assert.Equal(t, 10*time.Second, builder.ttl, "默认 TTL 应为 10 秒")
}

// TestRatelimit_TTL_DistinctIPs 验证不同 IP 的桶独立过期，互不影响。
func TestRatelimit_TTL_DistinctIPs(t *testing.T) {
	builder := Create(1, 1).WithByIP(true).WithTTL(100 * time.Millisecond)
	httpServer := newServerWith(builder)

	// IP1 请求 → 创建桶1
	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req1.RemoteAddr = "1.1.1.1:1111"
	w1 := httptest.NewRecorder()
	httpServer.ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusOK, w1.Code)

	// 50ms 后 IP2 请求 → 创建桶2
	time.Sleep(50 * time.Millisecond)
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.RemoteAddr = "2.2.2.2:2222"
	w2 := httptest.NewRecorder()
	httpServer.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)

	builder.bucketsMu.RLock()
	assert.Len(t, builder.buckets, 2)
	builder.bucketsMu.RUnlock()

	// 再等 80ms（总 130ms）：桶1 已过期（>100ms），桶2 未过期（80ms < 100ms）
	time.Sleep(80 * time.Millisecond)

	// IP1 再请求：桶1 被清理重建 → 200
	req3 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req3.RemoteAddr = "1.1.1.1:1111"
	w3 := httptest.NewRecorder()
	httpServer.ServeHTTP(w3, req3)
	assert.Equal(t, http.StatusOK, w3.Code, "IP1 桶过期后应获得新满桶")

	// IP2 立即请求：桶2 未过期，令牌已耗尽 → 429
	req4 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req4.RemoteAddr = "2.2.2.2:2222"
	w4 := httptest.NewRecorder()
	httpServer.ServeHTTP(w4, req4)
	assert.Equal(t, http.StatusTooManyRequests, w4.Code, "IP2 桶未过期，令牌仍为 0")
}
