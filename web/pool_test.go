package web

import (
	"net/http"
	"net/http/httptest"
	"runtime/debug"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestPool_FieldsResetBetweenRequests 验证 sync.Pool 复用 Context 时，
// 每个请求 handler 进入时各字段均为零值（无上一个请求的残留数据）。
//
// 通过禁用 GC 并发送多个顺序请求，确保 sync.Pool 在请求间稳定复用同一 ctx，
// 从而使 reset 的验证具备确定性。每轮 handler 入口检查所有 serve 不会触及的
// 字段（RespData、RespStatusCode、RespHeaders、RequestID、cacheQueryValues、
// done）以及 PathParams 是否为初始零值；随后主动写入这些字段以供下一轮校验。
func TestPool_FieldsResetBetweenRequests(t *testing.T) {
	// 禁用 GC，确保 sync.Pool 在请求间复用同一 ctx，使 reset 验证确定性
	old := debug.SetGCPercent(-1)
	defer debug.SetGCPercent(old)

	httpServer := NewHttpServer()

	var violations int32

	// /plain 路由无路径参数，进入 handler 时 PathParams 应为 nil
	httpServer.Get("/plain", func(ctx *Context) {
		// handler 入口：所有 serve 不会触及的字段必须为零值
		if ctx.RespData != nil {
			atomic.AddInt32(&violations, 1)
		}
		if ctx.RespStatusCode != 0 {
			atomic.AddInt32(&violations, 1)
		}
		if ctx.RespHeaders != nil {
			atomic.AddInt32(&violations, 1)
		}
		if ctx.PathParams != nil {
			atomic.AddInt32(&violations, 1)
		}
		if ctx.RequestID != "" {
			atomic.AddInt32(&violations, 1)
		}
		if ctx.cacheQueryValues != nil {
			atomic.AddInt32(&violations, 1)
		}
		if ctx.done {
			atomic.AddInt32(&violations, 1)
		}

		// 主动填充 cacheQueryValues、RespHeaders、RespData 等，
		// 下一轮 handler 入口应观察到它们被 reset 清空
		_ = ctx.QueryValue("k")
		_ = ctx.RespJson(http.StatusOK, map[string]string{"req": "ok"})
	})

	for i := 0; i < 100; i++ {
		req := httptest.NewRequest(http.MethodGet, "/plain?k=v", nil)
		w := httptest.NewRecorder()
		httpServer.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	}

	assert.Equal(t, int32(0), atomic.LoadInt32(&violations),
		"pool 复用的 ctx 在 handler 入口存在未 reset 的残留字段")
}

// TestPool_RespDataNoResidue_TR15_1 验证 TR-15.1：RespData 不残留。
// 第一个请求 handler 通过 RespJson 设置响应体；第二个请求 handler 进入时
// RespData 必须为 nil（已被 reset 清空），确保不会把上一个请求的响应体
// 误写回当前请求。
func TestPool_RespDataNoResidue_TR15_1(t *testing.T) {
	old := debug.SetGCPercent(-1)
	defer debug.SetGCPercent(old)

	httpServer := NewHttpServer()
	firstDone := false

	httpServer.Get("/hello", func(ctx *Context) {
		if !firstDone {
			_ = ctx.RespJson(http.StatusOK, map[string]string{"req": "first"})
			firstDone = true
			return
		}
		// 第二个请求入口：RespData 必须为 nil（已被 reset 清空）
		if ctx.RespData != nil {
			t.Fatalf("RespData 残留上一个请求的数据: %s", string(ctx.RespData))
		}
		_ = ctx.RespJson(http.StatusOK, map[string]string{"req": "second"})
	})

	req1 := httptest.NewRequest(http.MethodGet, "/hello", nil)
	w1 := httptest.NewRecorder()
	httpServer.ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusOK, w1.Code)
	assert.Contains(t, w1.Body.String(), "first")

	req2 := httptest.NewRequest(http.MethodGet, "/hello", nil)
	w2 := httptest.NewRecorder()
	httpServer.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)
	assert.Contains(t, w2.Body.String(), "second")
}

// TestPool_PathParamsNoResidue_TR15_2 验证 TR-15.2：PathParams 不残留。
// 第一个请求命中带路径参数的路由 /user/:id；第二个请求命中无参数路由 /ping，
// 第二个请求 handler 进入时 PathParams 必须为 nil。
func TestPool_PathParamsNoResidue_TR15_2(t *testing.T) {
	old := debug.SetGCPercent(-1)
	defer debug.SetGCPercent(old)

	httpServer := NewHttpServer()

	// 带路径参数的路由
	httpServer.Get("/user/:id", func(ctx *Context) {
		_ = ctx.RespJson(http.StatusOK, map[string]string{"id": ctx.PathValue("id").val})
	})

	// 无路径参数的路由：进入时 PathParams 必须为 nil
	plainCalled := false
	httpServer.Get("/ping", func(ctx *Context) {
		plainCalled = true
		if ctx.PathParams != nil {
			t.Fatalf("PathParams 残留上一个请求的数据: %v", ctx.PathParams)
		}
		_ = ctx.RespString(http.StatusOK, "pong")
	})

	// 第一个请求：带路径参数
	req1 := httptest.NewRequest(http.MethodGet, "/user/123", nil)
	w1 := httptest.NewRecorder()
	httpServer.ServeHTTP(w1, req1)
	assert.Equal(t, http.StatusOK, w1.Code)

	// 第二个请求：无路径参数，PathParams 应已 reset
	req2 := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w2 := httptest.NewRecorder()
	httpServer.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusOK, w2.Code)
	assert.True(t, plainCalled)
	assert.Contains(t, w2.Body.String(), "pong")
}
