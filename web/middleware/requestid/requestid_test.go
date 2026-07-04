package requestid

import (
	"Soil/web"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// newServer 是一个测试辅助函数，构建一个注册了 RequestID 中间件与 /test 路由的服务器。
// handler 会将 ctx.GetRequestID() 写入响应体，便于断言中间件设置的 RequestID 与 handler 中读取到的一致。
func newServer(t *testing.T, mdl web.Middleware) *web.HTTPServer {
	t.Helper()
	httpServer := web.NewHttpServer()
	httpServer.Use(mdl)
	httpServer.Get("/test", func(ctx *web.Context) {
		// 将 ctx.GetRequestID() 写入响应体，验证中间件与 handler 看到的是同一个值。
		ctx.RespString(http.StatusOK, ctx.GetRequestID())
	})
	return httpServer
}

// TR-5.1: 无 X-Request-Id 请求头时，响应头含 X-Request-Id，值为合法 UUID v4；
// 且 ctx.GetRequestID() 返回相同值。
func TestRequestID_NoHeader_GeneratesUUIDv4(t *testing.T) {
	httpServer := newServer(t, Create().Build())

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	httpServer.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	respRequestID := w.Header().Get("X-Request-Id")
	assert.NotEmpty(t, respRequestID, "响应头应包含 X-Request-Id")

	// 验证为合法 UUID（uuid.Parse 对 v4 生成的 UUID 不报错）。
	_, err := uuid.Parse(respRequestID)
	assert.NoError(t, err, "响应头 X-Request-Id 应为合法 UUID")

	// 响应体中写入的应是 handler 通过 ctx.GetRequestID() 读到的值，与响应头一致。
	bodyRequestID := w.Body.String()
	assert.Equal(t, respRequestID, bodyRequestID, "ctx.GetRequestID() 应与响应头 X-Request-Id 一致")
}

// TR-5.2: 请求带 X-Request-Id: abc-123 时，响应头 X-Request-Id 为 "abc-123"，
// 且 ctx.GetRequestID() 为 "abc-123"。
func TestRequestID_WithHeader_ReusesIncomingValue(t *testing.T) {
	httpServer := newServer(t, Create().Build())

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Request-Id", "abc-123")
	w := httptest.NewRecorder()

	httpServer.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	respRequestID := w.Header().Get("X-Request-Id")
	assert.Equal(t, "abc-123", respRequestID, "响应头应沿用请求中的 X-Request-Id")

	// 响应体即 ctx.GetRequestID() 的值。
	bodyRequestID := w.Body.String()
	assert.Equal(t, "abc-123", bodyRequestID, "ctx.GetRequestID() 应为请求中的 X-Request-Id 值")
}

// TR-5.3: 两次请求（无 X-Request-Id 头）的 RequestID 不同。
func TestRequestID_TwoRequests_GenerateDifferentIDs(t *testing.T) {
	httpServer := newServer(t, Create().Build())

	// 第一次请求
	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	w1 := httptest.NewRecorder()
	httpServer.ServeHTTP(w1, req1)
	id1 := w1.Header().Get("X-Request-Id")

	// 第二次请求
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	w2 := httptest.NewRecorder()
	httpServer.ServeHTTP(w2, req2)
	id2 := w2.Header().Get("X-Request-Id")

	assert.NotEmpty(t, id1)
	assert.NotEmpty(t, id2)
	assert.NotEqual(t, id1, id2, "两次新生成的 RequestID 应不同")
}

// 补充：WithHeaderName 自定义头名称时，应使用自定义头读写 RequestID。
func TestRequestID_CustomHeaderName(t *testing.T) {
	httpServer := newServer(t, Create().WithHeaderName("X-Trace-Id").Build())

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Trace-Id", "trace-001")
	w := httptest.NewRecorder()

	httpServer.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "trace-001", w.Header().Get("X-Trace-Id"))
	// 默认头名称不应被设置。
	assert.Empty(t, w.Header().Get("X-Request-Id"))
	// 响应体即 ctx.GetRequestID() 的值。
	assert.Equal(t, "trace-001", w.Body.String())
}
