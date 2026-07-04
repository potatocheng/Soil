package ratelimit

import (
	"Soil/web"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

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
