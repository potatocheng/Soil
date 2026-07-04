package gzip

import (
	"Soil/web"
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// genString 生成指定长度的重复字符串用于测试。
func genString(n int) string {
	var sb strings.Builder
	for sb.Len() < n {
		sb.WriteString("abcdefghijklmnopqrstuvwxyz0123456789")
	}
	return sb.String()[:n]
}

// decompress 使用 gzip reader 解压响应体。
func decompress(t *testing.T, data []byte) []byte {
	t.Helper()
	r, err := gzip.NewReader(bytes.NewReader(data))
	assert.NoError(t, err)
	bs, err := io.ReadAll(r)
	assert.NoError(t, err)
	assert.NoError(t, r.Close())
	return bs
}

// TR-9.1: 2KB 文本响应 + Accept-Encoding: gzip -> 响应头 Content-Encoding: gzip，解压后为原文
func TestGzip_CompressLargeText(t *testing.T) {
	httpServer := web.NewHttpServer()
	gzipMiddleware := Create().Build()
	httpServer.Use(gzipMiddleware)

	original := genString(2048)
	httpServer.Get("/big", func(ctx *web.Context) {
		err := ctx.RespString(http.StatusOK, original)
		assert.NoError(t, err)
	})

	req := httptest.NewRequest(http.MethodGet, "/big", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()

	httpServer.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "gzip", w.Header().Get("Content-Encoding"))

	// 压缩后体积应小于原文
	assert.Less(t, w.Body.Len(), len(original))

	// 解压后应与原文一致
	decompressed := decompress(t, w.Body.Bytes())
	assert.Equal(t, original, string(decompressed))
}

// TR-9.2: 500B 响应（小于阈值 1024）不压缩，无 Content-Encoding 头
func TestGzip_SkipSmallResponse(t *testing.T) {
	httpServer := web.NewHttpServer()
	gzipMiddleware := Create().Build()
	httpServer.Use(gzipMiddleware)

	original := genString(500)
	httpServer.Get("/small", func(ctx *web.Context) {
		err := ctx.RespString(http.StatusOK, original)
		assert.NoError(t, err)
	})

	req := httptest.NewRequest(http.MethodGet, "/small", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()

	httpServer.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	// 小于阈值不压缩
	assert.Empty(t, w.Header().Get("Content-Encoding"))
	// 响应体应为原文
	assert.Equal(t, original, w.Body.String())
}

// TR-9.3: 不带 Accept-Encoding: gzip 的请求不压缩
func TestGzip_NoAcceptEncoding(t *testing.T) {
	httpServer := web.NewHttpServer()
	gzipMiddleware := Create().Build()
	httpServer.Use(gzipMiddleware)

	original := genString(2048)
	httpServer.Get("/big", func(ctx *web.Context) {
		err := ctx.RespString(http.StatusOK, original)
		assert.NoError(t, err)
	})

	req := httptest.NewRequest(http.MethodGet, "/big", nil)
	// 不设置 Accept-Encoding
	w := httptest.NewRecorder()

	httpServer.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, w.Header().Get("Content-Encoding"))
	assert.Equal(t, original, w.Body.String())
}

// TR-9.4: Content-Type 为 image/png 的响应不压缩
func TestGzip_SkipImageContentType(t *testing.T) {
	httpServer := web.NewHttpServer()
	gzipMiddleware := Create().Build()
	httpServer.Use(gzipMiddleware)

	// 构造一段模拟 png 数据（仅用于测试类型判断，不需要真实 png）
	pngData := bytes.Repeat([]byte{0x89, 0x50, 0x4E, 0x47}, 600) // > 1024 字节
	httpServer.Get("/img", func(ctx *web.Context) {
		ctx.SetHeader("Content-Type", "image/png")
		ctx.RespStatusCode = http.StatusOK
		ctx.RespData = pngData
	})

	req := httptest.NewRequest(http.MethodGet, "/img", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()

	httpServer.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	// image/png 不压缩
	assert.Empty(t, w.Header().Get("Content-Encoding"))
	assert.Equal(t, pngData, w.Body.Bytes())
}

// 额外测试：JSON 响应可以被压缩
func TestGzip_CompressJSON(t *testing.T) {
	httpServer := web.NewHttpServer()
	gzipMiddleware := Create().Build()
	httpServer.Use(gzipMiddleware)

	original := genString(2048)
	httpServer.Get("/json", func(ctx *web.Context) {
		err := ctx.RespJson(http.StatusOK, map[string]string{"data": original})
		assert.NoError(t, err)
	})

	req := httptest.NewRequest(http.MethodGet, "/json", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()

	httpServer.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "gzip", w.Header().Get("Content-Encoding"))
	// Content-Length 应已被删除
	assert.Empty(t, w.Header().Get("Content-Length"))

	decompressed := decompress(t, w.Body.Bytes())
	assert.Contains(t, string(decompressed), original)
}

// 额外测试：链式配置 WithMinSize / WithLevel 生效
func TestGzip_ChainedConfig(t *testing.T) {
	httpServer := web.NewHttpServer()
	gzipMiddleware := Create().WithMinSize(100).WithLevel(gzip.BestSpeed).Build()
	httpServer.Use(gzipMiddleware)

	original := genString(200)
	httpServer.Get("/mid", func(ctx *web.Context) {
		err := ctx.RespString(http.StatusOK, original)
		assert.NoError(t, err)
	})

	req := httptest.NewRequest(http.MethodGet, "/mid", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()

	httpServer.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "gzip", w.Header().Get("Content-Encoding"))
	assert.Equal(t, "Accept-Encoding", w.Header().Get("Vary"))

	decompressed := decompress(t, w.Body.Bytes())
	assert.Equal(t, original, string(decompressed))
}

// 额外测试：Vary 头被正确设置
func TestGzip_SetsVaryHeader(t *testing.T) {
	httpServer := web.NewHttpServer()
	gzipMiddleware := Create().Build()
	httpServer.Use(gzipMiddleware)

	original := genString(2048)
	httpServer.Get("/vary", func(ctx *web.Context) {
		err := ctx.RespString(http.StatusOK, original)
		assert.NoError(t, err)
	})

	req := httptest.NewRequest(http.MethodGet, "/vary", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()

	httpServer.ServeHTTP(w, req)

	assert.Equal(t, "Accept-Encoding", w.Header().Get("Vary"))
}
