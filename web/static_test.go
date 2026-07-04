package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupStaticDir 创建一个临时目录结构用于静态文件服务测试：
//
//	tmpDir/
//	  secret.txt       ("TOP SECRET")      —— 静态目录之外，路径穿越不应能访问
//	  static/
//	    test.txt       ("hello static")    —— 静态目录内，应可被访问
//	    sub/
//	      nested.txt   ("nested file")     —— 子目录文件
//
// 返回静态目录（tmpDir/static）的绝对路径，并在测试结束后自动清理。
func setupStaticDir(t *testing.T) (staticDir string) {
	t.Helper()
	tmpDir := t.TempDir()

	// 静态目录外的敏感文件（用于验证路径穿越防护）
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "secret.txt"), []byte("TOP SECRET"), 0644))

	staticDir = filepath.Join(tmpDir, "static")
	require.NoError(t, os.MkdirAll(filepath.Join(staticDir, "sub"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(staticDir, "test.txt"), []byte("hello static"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(staticDir, "sub", "nested.txt"), []byte("nested file"), 0644))

	return staticDir
}

// TR-12.1: 请求 /assets/test.txt 返回文件内容 "hello static"，
// Content-Type 为 text/plain（http.FileServer 根据扩展名自动推断）。
func TestStatic_ServeFile(t *testing.T) {
	staticDir := setupStaticDir(t)
	httpServer := NewHttpServer()
	httpServer.Static("/assets", staticDir)

	req := httptest.NewRequest(http.MethodGet, "/assets/test.txt", nil)
	w := httptest.NewRecorder()
	httpServer.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "hello static", w.Body.String())
	// http.FileServer 通过 mime 推断 .txt 为 text/plain
	contentType := w.Header().Get("Content-Type")
	assert.Contains(t, contentType, "text/plain")
}

// TR-12.1 补充：子目录文件也能正确访问。
func TestStatic_ServeNestedFile(t *testing.T) {
	staticDir := setupStaticDir(t)
	httpServer := NewHttpServer()
	httpServer.Static("/assets", staticDir)

	req := httptest.NewRequest(http.MethodGet, "/assets/sub/nested.txt", nil)
	w := httptest.NewRecorder()
	httpServer.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "nested file", w.Body.String())
}

// TR-12.2: 路径穿越防护。请求 /assets/../secret.txt 试图访问静态目录外的
// secret.txt，http.FileServer 通过 path.Clean 清理 ".."，不会返回上级目录内容，
// 而是返回 404（清理后路径在静态目录内不存在）。
func TestStatic_PathTraversalProtection(t *testing.T) {
	staticDir := setupStaticDir(t)
	httpServer := NewHttpServer()
	httpServer.Static("/assets", staticDir)

	req := httptest.NewRequest(http.MethodGet, "/assets/../secret.txt", nil)
	w := httptest.NewRecorder()
	httpServer.ServeHTTP(w, req)

	// 不能返回 200，也不能泄露 secret.txt 的内容
	assert.NotEqual(t, http.StatusOK, w.Code)
	assert.NotContains(t, w.Body.String(), "TOP SECRET")
	// 期望为 404（或 403）
	assert.True(t, w.Code == http.StatusNotFound || w.Code == http.StatusForbidden,
		"期望 404 或 403，实际 %d", w.Code)
}

// TR-12.4: 请求不存在的文件 /assets/notexist.txt 返回 404。
func TestStatic_NotFound(t *testing.T) {
	staticDir := setupStaticDir(t)
	httpServer := NewHttpServer()
	httpServer.Static("/assets", staticDir)

	req := httptest.NewRequest(http.MethodGet, "/assets/notexist.txt", nil)
	w := httptest.NewRecorder()
	httpServer.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TR-12 补充：静态路由与普通路由可共存，且互不影响。
func TestStatic_CoexistWithNormalRoute(t *testing.T) {
	staticDir := setupStaticDir(t)
	httpServer := NewHttpServer()

	httpServer.Get("/api/health", func(ctx *Context) {
		ctx.RespString(http.StatusOK, "ok")
	})
	httpServer.Static("/assets", staticDir)

	// 普通路由正常
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	w := httptest.NewRecorder()
	httpServer.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "ok", w.Body.String())

	// 静态文件正常
	req = httptest.NewRequest(http.MethodGet, "/assets/test.txt", nil)
	w = httptest.NewRecorder()
	httpServer.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "hello static", w.Body.String())
}
