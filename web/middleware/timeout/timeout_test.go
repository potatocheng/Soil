package timeout

import (
	"Soil/web"
	"Soil/web/middleware/recovery"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TR-8.1: 配置 50ms 超时，handler 中 time.Sleep(200ms)，
// 请求返回 503，响应体含 "Service Unavailable"。
func TestTimeout_HandlerExceedsTimeout(t *testing.T) {
	httpServer := web.NewHttpServer()
	middleware := Create(50 * time.Millisecond).Build()
	httpServer.Use(middleware)
	httpServer.Get("/slow", func(ctx *web.Context) {
		time.Sleep(200 * time.Millisecond)
		// 超时后 handler 仍会执行到这里，但其写入的字段不会被回写
		ctx.RespString(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/slow", nil)
	w := httptest.NewRecorder()
	httpServer.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Contains(t, w.Body.String(), "Service Unavailable")
}

// TR-8.2: 配置 200ms 超时，handler 中 time.Sleep(50ms)，
// 正常返回原状态码 200。
func TestTimeout_HandlerWithinTimeout(t *testing.T) {
	httpServer := web.NewHttpServer()
	middleware := Create(200 * time.Millisecond).Build()
	httpServer.Use(middleware)
	httpServer.Get("/slow", func(ctx *web.Context) {
		time.Sleep(50 * time.Millisecond)
		ctx.RespString(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/slow", nil)
	w := httptest.NewRecorder()
	httpServer.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "ok")
}

// TR-8.3: 超时后 handler 仍执行时不产生 panic
// （handler 中 select 监听 ctx.Req.Context().Done() 主动退出）。
func TestTimeout_HandlerExitsOnContextDone(t *testing.T) {
	httpServer := web.NewHttpServer()
	middleware := Create(50 * time.Millisecond).Build()
	httpServer.Use(middleware)
	httpServer.Get("/slow", func(ctx *web.Context) {
		select {
		case <-time.After(300 * time.Millisecond):
			// 不会走到这里：超时前 context 已取消
			ctx.RespString(http.StatusOK, "ok")
		case <-ctx.Req.Context().Done():
			// 感知到超时，主动退出，避免超时后继续操作 ctx
			return
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/slow", nil)
	w := httptest.NewRecorder()

	assert.NotPanics(t, func() {
		httpServer.ServeHTTP(w, req)
	})

	// 超时分支生效，返回 503
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Contains(t, w.Body.String(), "Service Unavailable")
}

// TR-8.4: timeout + recovery 组合时 handler panic 应被捕获返回 500，而非崩溃进程。
//
// timeout 中间件在独立 goroutine 中执行 handler，handler 内 panic 在子 goroutine
// 中产生，外层 recovery（位于主 goroutine）的 defer recover() 无法跨 goroutine
// 捕获。timeout 中间件在 goroutine 内部 defer recover() 并通过 panicCh 将 panic
// 传回主 goroutine 设置 500，由 flashResp 统一写出，避免进程崩溃。
func TestTimeout_PanicRecovery(t *testing.T) {
	server := web.NewHttpServer()
	// recovery 在外层（先 Use），timeout 在内层（后 Use）
	server.Use(recovery.Create().Build())
	server.Use(Create(1 * time.Second).Build())
	server.Get("/panic", func(ctx *web.Context) {
		panic("test panic")
	})

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	w := httptest.NewRecorder()

	// 不应 panic（进程不崩溃）
	assert.NotPanics(t, func() {
		server.ServeHTTP(w, req)
	})

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "Internal Server Error")
}
