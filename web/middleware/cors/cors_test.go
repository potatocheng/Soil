package cors

import (
	"Soil/web"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// newServer 是一个测试辅助函数，构建一个注册了 CORS 中间件与 /test 路由的服务器。
func newServer(t *testing.T, mdl web.Middleware) *web.HTTPServer {
	t.Helper()
	httpServer := web.NewHttpServer()
	httpServer.Use(mdl)
	httpServer.Get("/test", func(ctx *web.Context) {
		ctx.RespJson(http.StatusOK, map[string]string{"msg": "ok"})
	})
	return httpServer
}

// TR-6.1: OPTIONS 预检请求带允许的 Origin 返回 204，含 Access-Control-Allow-Origin。
func TestCORS_PreflightAllowedOrigin(t *testing.T) {
	httpServer := newServer(t, Create().
		AllowOrigins("https://example.com").
		Build())

	req := httptest.NewRequest(http.MethodOptions, "/test", nil)
	req.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()

	httpServer.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "https://example.com", w.Header().Get("Access-Control-Allow-Origin"))
	// 默认允许的方法也应写入响应头。
	assert.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "GET")
	assert.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), "OPTIONS")
}

// TR-6.2: 不允许的 Origin 响应不含 Access-Control-Allow-Origin 头。
func TestCORS_PreflightDisallowedOrigin(t *testing.T) {
	httpServer := newServer(t, Create().
		AllowOrigins("https://example.com").
		Build())

	req := httptest.NewRequest(http.MethodOptions, "/test", nil)
	req.Header.Set("Origin", "https://evil.com")
	w := httptest.NewRecorder()

	httpServer.ServeHTTP(w, req)

	_, exists := w.Header()["Access-Control-Allow-Origin"]
	assert.False(t, exists, "不允许的 Origin 不应返回 Access-Control-Allow-Origin 头")
}

// TR-6.3: AllowCredentials(true) 时响应含 Access-Control-Allow-Credentials: true。
func TestCORS_AllowCredentials(t *testing.T) {
	httpServer := newServer(t, Create().
		AllowOrigins("https://example.com").
		AllowCredentials(true).
		Build())

	req := httptest.NewRequest(http.MethodOptions, "/test", nil)
	req.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()

	httpServer.ServeHTTP(w, req)

	assert.Equal(t, "true", w.Header().Get("Access-Control-Allow-Credentials"))
}

// TR-6.4: 普通 GET 请求响应头含 CORS 头。
func TestCORS_NormalGet(t *testing.T) {
	httpServer := newServer(t, Create().
		AllowOrigins("https://example.com").
		Build())

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()

	httpServer.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "https://example.com", w.Header().Get("Access-Control-Allow-Origin"))
	assert.Contains(t, w.Body.String(), "ok")
}

// TR-6.5: AllowOrigins("*") 时允许任意 Origin。
func TestCORS_WildcardOrigin(t *testing.T) {
	httpServer := newServer(t, Create().
		AllowOrigins("*").
		Build())

	req := httptest.NewRequest(http.MethodOptions, "/test", nil)
	req.Header.Set("Origin", "https://anything.com")
	w := httptest.NewRecorder()

	httpServer.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "*", w.Header().Get("Access-Control-Allow-Origin"))
}

// 互斥校验：AllowOrigins("*") + AllowCredentials(true) 调用 Build 时 panic。
func TestCORS_WildcardAndCredentialsMutuallyExclusive(t *testing.T) {
	builder := Create().
		AllowOrigins("*").
		AllowCredentials(true)

	assert.Panics(t, func() {
		builder.Build()
	})
}

// 补充：不带 Origin 头的普通请求不应设置 CORS 头，且正常返回。
func TestCORS_NoOriginHeader(t *testing.T) {
	httpServer := newServer(t, Create().
		AllowOrigins("https://example.com").
		Build())

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	httpServer.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	_, exists := w.Header()["Access-Control-Allow-Origin"]
	assert.False(t, exists, "无 Origin 头的请求不应返回 CORS 头")
}

// 补充：ExposeHeaders 配置在普通请求中应写入 Access-Control-Expose-Headers。
func TestCORS_ExposeHeaders(t *testing.T) {
	httpServer := newServer(t, Create().
		AllowOrigins("https://example.com").
		ExposeHeaders("X-Request-Id", "X-Trace-Id").
		Build())

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "https://example.com")
	w := httptest.NewRecorder()

	httpServer.ServeHTTP(w, req)

	assert.Equal(t, "X-Request-Id, X-Trace-Id", w.Header().Get("Access-Control-Expose-Headers"))
}
