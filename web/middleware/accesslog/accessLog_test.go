package accesslog

import (
	"Soil/web"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAccessLog(t *testing.T) {
	httpServer := web.NewHttpServer()

	var capturedLog string
	logBuilder := Create().WithLogFunc(func(accessLog string) {
		capturedLog = accessLog
	})
	logMiddleware := logBuilder.Build()

	httpServer.Use(logMiddleware)
	httpServer.Get("/", func(ctx *web.Context) {
		ctx.RespJson(http.StatusOK, map[string]string{"message": "hello world"})
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	httpServer.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, capturedLog, "GET")
	assert.Contains(t, capturedLog, "/")
}

func TestAccessLog_PathParam(t *testing.T) {
	httpServer := web.NewHttpServer()

	var capturedLog string
	logBuilder := Create().WithLogFunc(func(accessLog string) {
		capturedLog = accessLog
	})
	logMiddleware := logBuilder.Build()

	httpServer.Use(logMiddleware)
	httpServer.Get("/user/:id", func(ctx *web.Context) {
		ctx.RespJson(http.StatusOK, map[string]string{"id": ctx.PathValue("id").Value()})
	})

	req := httptest.NewRequest(http.MethodGet, "/user/123", nil)
	w := httptest.NewRecorder()

	httpServer.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, capturedLog, "/user/:id")
	assert.Contains(t, capturedLog, "123")
}

// TestAccessLog_NewFields 验证 TR-4.1：日志 JSON 解析后包含新增字段。
func TestAccessLog_NewFields(t *testing.T) {
	httpServer := web.NewHttpServer()

	var capturedLog string
	logBuilder := Create().WithLogFunc(func(accessLog string) {
		capturedLog = accessLog
	})
	httpServer.Use(logBuilder.Build())

	httpServer.Get("/", func(ctx *web.Context) {
		ctx.RespJson(http.StatusOK, map[string]string{"message": "hello"})
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("User-Agent", "test-agent")
	w := httptest.NewRecorder()
	httpServer.ServeHTTP(w, req)

	var m map[string]interface{}
	err := json.Unmarshal([]byte(capturedLog), &m)
	assert.NoError(t, err)

	// TR-4.1：包含全部新增字段
	assert.Contains(t, m, "status")
	assert.Contains(t, m, "latency_ms")
	assert.Contains(t, m, "resp_size")
	assert.Contains(t, m, "client_ip")
	assert.Contains(t, m, "user_agent")
}

// TestAccessLog_StatusCodes 验证 TR-4.2：handler 返回 200 时 status 为 200，返回 404 时 status 为 404。
func TestAccessLog_StatusCodes(t *testing.T) {
	t.Run("200", func(t *testing.T) {
		httpServer := web.NewHttpServer()

		var capturedLog string
		logBuilder := Create().WithLogFunc(func(accessLog string) {
			capturedLog = accessLog
		})
		httpServer.Use(logBuilder.Build())

		httpServer.Get("/", func(ctx *web.Context) {
			ctx.RespJson(http.StatusOK, map[string]string{"message": "ok"})
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()
		httpServer.ServeHTTP(w, req)

		var m map[string]interface{}
		err := json.Unmarshal([]byte(capturedLog), &m)
		assert.NoError(t, err)
		assert.Equal(t, float64(200), m["status"])
	})

	t.Run("404", func(t *testing.T) {
		httpServer := web.NewHttpServer()

		var capturedLog string
		logBuilder := Create().WithLogFunc(func(accessLog string) {
			capturedLog = accessLog
		})
		httpServer.Use(logBuilder.Build())

		httpServer.Get("/missing", func(ctx *web.Context) {
			ctx.RespJson(http.StatusNotFound, map[string]string{"error": "not found"})
		})

		req := httptest.NewRequest(http.MethodGet, "/missing", nil)
		w := httptest.NewRecorder()
		httpServer.ServeHTTP(w, req)

		var m map[string]interface{}
		err := json.Unmarshal([]byte(capturedLog), &m)
		assert.NoError(t, err)
		assert.Equal(t, float64(404), m["status"])
	})
}

// TestAccessLog_ClientIP 验证 TR-4.3：X-Forwarded-For 取第一个 IP。
func TestAccessLog_ClientIP(t *testing.T) {
	httpServer := web.NewHttpServer()

	var capturedLog string
	logBuilder := Create().WithLogFunc(func(accessLog string) {
		capturedLog = accessLog
	})
	httpServer.Use(logBuilder.Build())

	httpServer.Get("/", func(ctx *web.Context) {
		ctx.RespJson(http.StatusOK, map[string]string{"message": "hello"})
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")
	w := httptest.NewRecorder()
	httpServer.ServeHTTP(w, req)

	var m map[string]interface{}
	err := json.Unmarshal([]byte(capturedLog), &m)
	assert.NoError(t, err)
	assert.Equal(t, "1.2.3.4", m["client_ip"])
}

// TestAccessLog_Latency 验证 TR-4.4：latency_ms >= 0。
func TestAccessLog_Latency(t *testing.T) {
	httpServer := web.NewHttpServer()

	var capturedLog string
	logBuilder := Create().WithLogFunc(func(accessLog string) {
		capturedLog = accessLog
	})
	httpServer.Use(logBuilder.Build())

	httpServer.Get("/", func(ctx *web.Context) {
		ctx.RespJson(http.StatusOK, map[string]string{"message": "hello"})
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	httpServer.ServeHTTP(w, req)

	var m map[string]interface{}
	err := json.Unmarshal([]byte(capturedLog), &m)
	assert.NoError(t, err)

	latency, ok := m["latency_ms"].(float64)
	assert.True(t, ok)
	assert.GreaterOrEqual(t, int64(latency), int64(0))
}
