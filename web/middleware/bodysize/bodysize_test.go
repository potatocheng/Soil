package bodysize

import (
	"Soil/web"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TR-10.1: 配置 maxBytes=1024，请求体 2048 字节，返回 413
func TestBodySize_TooLarge(t *testing.T) {
	httpServer := web.NewHttpServer()
	middleware := Create(1024).Build()
	httpServer.Use(middleware)
	httpServer.Post("/upload", func(ctx *web.Context) {
		data, _ := io.ReadAll(ctx.Req.Body)
		ctx.RespString(http.StatusOK, "got %d bytes", len(data))
	})

	req := httptest.NewRequest(http.MethodPost, "/upload", strings.NewReader(strings.Repeat("a", 2048)))
	w := httptest.NewRecorder()
	httpServer.ServeHTTP(w, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
	assert.Equal(t, "Request Entity Too Large", w.Body.String())
}

// TR-10.2: 配置 maxBytes=1024，请求体 512 字节，正常处理（handler 读 body 成功返回 200）
func TestBodySize_OK(t *testing.T) {
	httpServer := web.NewHttpServer()
	middleware := Create(1024).Build()
	httpServer.Use(middleware)
	httpServer.Post("/upload", func(ctx *web.Context) {
		data, err := io.ReadAll(ctx.Req.Body)
		if err != nil {
			ctx.RespString(http.StatusInternalServerError, "read err: %v", err)
			return
		}
		ctx.RespString(http.StatusOK, "got %d bytes", len(data))
	})

	req := httptest.NewRequest(http.MethodPost, "/upload", strings.NewReader(strings.Repeat("a", 512)))
	w := httptest.NewRecorder()
	httpServer.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "512")
}

// TR-10.3: 无 body 的 GET 请求不受影响（ContentLength=0，正常处理）
func TestBodySize_NoBody(t *testing.T) {
	httpServer := web.NewHttpServer()
	middleware := Create(1024).Build()
	httpServer.Use(middleware)
	httpServer.Get("/upload", func(ctx *web.Context) {
		ctx.RespString(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/upload", nil)
	w := httptest.NewRecorder()
	httpServer.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "ok", w.Body.String())
}
