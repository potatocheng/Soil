package web_test

import (
	"Soil/web"
	"Soil/web/middleware/accesslog"
	"Soil/web/middleware/cors"
	mwgzip "Soil/web/middleware/gzip"
	"Soil/web/middleware/ratelimit"
	"Soil/web/middleware/recovery"
	"Soil/web/middleware/requestid"
	"Soil/web/middleware/timeout"
	"bytes"
	cgzip "compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestIntegration_MiddlewareChain 端到端集成测试：组合 recovery / requestid /
// accesslog / cors / ratelimit / timeout / gzip 七个中间件，验证它们在协同工作时的
// 行为符合预期。
//
// 中间件注册顺序（Use 顺序）即执行顺序：ServeHTTP 中反向包裹，
// 第一个 Use 的中间件在最外层最先执行。
//
// timeout 与 recovery 协同：timeout 中间件在 goroutine 内部 defer recover() 捕获
// handler panic，并通过 panicCh 将 panic 传回主 goroutine 设置 500，避免跨
// goroutine panic 导致进程崩溃。详见 TestIntegration_TimeoutRecoveryCoordination。
func TestIntegration_MiddlewareChain(t *testing.T) {
	server := web.NewHttpServer()

	// accesslog 日志捕获（跨子测试共享，加锁保护）
	var (
		capturedLog string
		logMu       sync.Mutex
	)
	captureLog := func(s string) {
		logMu.Lock()
		capturedLog = s
		logMu.Unlock()
	}

	// 按顺序注册中间件：recovery -> requestid -> accesslog -> cors -> ratelimit -> timeout -> gzip
	server.Use(recovery.Create().Build())
	server.Use(requestid.Create().Build())
	server.Use(accesslog.Create().WithLogFunc(captureLog).Build())
	server.Use(cors.Create().AllowOrigins("https://example.com").Build())
	server.Use(ratelimit.Create(100, 100).Build())
	server.Use(timeout.Create(5 * time.Second).Build())
	server.Use(mwgzip.Create().Build())

	// 路由分组
	api := server.Group("/api/v1")
	api.Get("/users", func(ctx *web.Context) {
		_ = ctx.RespJson(http.StatusOK, map[string]any{"users": []string{"alice", "bob"}})
	})
	// /large 返回足够大的 JSON 以触发 gzip 压缩（> 1024 字节）
	api.Get("/large", func(ctx *web.Context) {
		users := make([]string, 200)
		for i := range users {
			users[i] = fmt.Sprintf("user_%d", i)
		}
		_ = ctx.RespJson(http.StatusOK, map[string]any{"users": users})
	})

	// 场景 A: 正常 GET 请求，返回 200，含 X-Request-Id 与 CORS 头
	t.Run("NormalRequest", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		req.Header.Set("Origin", "https://example.com")
		w := httptest.NewRecorder()
		server.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.NotEmpty(t, w.Header().Get("X-Request-Id"))
		assert.Equal(t, "https://example.com", w.Header().Get("Access-Control-Allow-Origin"))
		// /users 响应体较小（< 1024B），gzip 不会压缩，故无 Content-Encoding
		assert.Empty(t, w.Header().Get("Content-Encoding"))

		// 验证 accesslog 含 status 字段
		logMu.Lock()
		log := capturedLog
		logMu.Unlock()
		assert.Contains(t, log, `"status":200`)
	})

	// 场景 B: POST 一个只注册了 GET 的路径 -> 405, Allow 头含 GET
	t.Run("MethodNotAllowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/users", nil)
		w := httptest.NewRecorder()
		server.ServeHTTP(w, req)

		assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
		assert.Contains(t, w.Header().Get("Allow"), http.MethodGet)
	})

	// 场景 C: 请求不存在的路径 -> 404
	t.Run("NotFound", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/nonexistent", nil)
		w := httptest.NewRecorder()
		server.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	// 场景 D: handler panic, recovery 捕获返回 500
	//
	// 本场景使用不含 timeout 的中间件链直接验证 recovery 中间件自身的 panic
	// 捕获能力。timeout + recovery 组合下的 panic 处理（timeout 在 goroutine 内
	// recover 并设置 500）由 TestIntegration_TimeoutRecoveryCoordination 单独覆盖。
	t.Run("PanicRecovery", func(t *testing.T) {
		serverNoTimeout := web.NewHttpServer()
		serverNoTimeout.Use(recovery.Create().Build())
		serverNoTimeout.Use(requestid.Create().Build())
		serverNoTimeout.Use(accesslog.Create().WithLogFunc(captureLog).Build())
		serverNoTimeout.Use(cors.Create().AllowOrigins("https://example.com").Build())
		serverNoTimeout.Use(ratelimit.Create(100, 100).Build())
		// 注意：此处不注册 timeout 中间件
		serverNoTimeout.Use(mwgzip.Create().Build())

		api2 := serverNoTimeout.Group("/api/v1")
		api2.Get("/panic", func(ctx *web.Context) {
			panic("test panic")
		})

		req := httptest.NewRequest(http.MethodGet, "/api/v1/panic", nil)
		w := httptest.NewRecorder()
		serverNoTimeout.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "Internal Server Error")
	})

	// 场景 E: OPTIONS 预检请求带 Origin -> 204 + CORS 头
	t.Run("CORSPreflight", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "/api/v1/users", nil)
		req.Header.Set("Origin", "https://example.com")
		w := httptest.NewRecorder()
		server.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNoContent, w.Code)
		assert.Equal(t, "https://example.com", w.Header().Get("Access-Control-Allow-Origin"))
		assert.Contains(t, w.Header().Get("Access-Control-Allow-Methods"), http.MethodGet)
		assert.NotEmpty(t, w.Header().Get("Access-Control-Max-Age"))
		// requestid 已在 cors 之前执行，X-Request-Id 应存在
		assert.NotEmpty(t, w.Header().Get("X-Request-Id"))
	})

	// 场景 F: 大响应 + Accept-Encoding: gzip -> 响应被压缩，解压后与原文一致
	//
	// 同时验证 accesslog 协同行为：accesslog 在 gzip 外层注册（先于 gzip），
	// 其 defer 在 gzip 压缩 RespData 之后执行，因此 resp_size 为压缩后大小。
	t.Run("GzipCompression", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/large", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		w := httptest.NewRecorder()
		server.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "gzip", w.Header().Get("Content-Encoding"))
		assert.Equal(t, "Accept-Encoding", w.Header().Get("Vary"))

		// 解压验证
		r, err := cgzip.NewReader(bytes.NewReader(w.Body.Bytes()))
		assert.NoError(t, err)
		decompressed, err := io.ReadAll(r)
		assert.NoError(t, err)
		assert.NoError(t, r.Close())

		var result map[string]any
		assert.NoError(t, json.Unmarshal(decompressed, &result))
		users, ok := result["users"].([]any)
		assert.True(t, ok)
		assert.Equal(t, 200, len(users))

		// 压缩后体积应小于原文
		assert.Less(t, w.Body.Len(), len(decompressed))

		// accesslog 的 resp_size 应为压缩后大小（accesslog 在 gzip 外层，
		// defer 执行时 gzip 已压缩 RespData）
		logMu.Lock()
		log := capturedLog
		logMu.Unlock()
		var logMap map[string]any
		assert.NoError(t, json.Unmarshal([]byte(log), &logMap))
		respSize, ok := logMap["resp_size"].(float64)
		assert.True(t, ok, "resp_size 应为数字")
		assert.Equal(t, w.Body.Len(), int(respSize),
			"accesslog 的 resp_size 应为压缩后大小")
	})
}

// TestIntegration_TimeoutRecoveryCoordination 验证 timeout 与 recovery 中间件的
// 协同：timeout 在独立 goroutine 中执行 handler，handler 内 panic 在子 goroutine
// 中产生，外层 recovery（位于主 goroutine）无法跨 goroutine 捕获。timeout 中间件
// 在 goroutine 内部 defer recover() 并通过 panicCh 将 panic 传回主 goroutine 设置
// 500 状态码，从而避免进程崩溃。
//
// 本测试通过子进程方式验证：子进程以 TEST_TIMEOUT_PANIC=1 环境变量运行时执行会
// panic 的场景，父进程断言子进程正常退出（exit code 0）且响应为 500，确保修复后
// 不再崩溃。
func TestIntegration_TimeoutRecoveryCoordination(t *testing.T) {
	if os.Getenv("TEST_TIMEOUT_PANIC") == "1" {
		// 子进程模式：构建含 timeout + recovery 的链，handler panic 应被
		// timeout 中间件在 goroutine 内 recover，返回 500 而非崩溃。
		server := web.NewHttpServer()
		server.Use(recovery.Create().Build())
		server.Use(timeout.Create(5 * time.Second).Build())
		server.Get("/panic", func(ctx *web.Context) {
			panic("goroutine panic")
		})
		req := httptest.NewRequest(http.MethodGet, "/panic", nil)
		w := httptest.NewRecorder()
		server.ServeHTTP(w, req)
		// 输出状态码供父进程校验
		fmt.Println("STATUS:", w.Code)
		return
	}

	// 父进程模式：启动子进程执行上述场景，预期子进程正常退出（不崩溃）
	cmd := exec.Command(os.Args[0], "-test.run=^TestIntegration_TimeoutRecoveryCoordination$")
	cmd.Env = append(os.Environ(), "TEST_TIMEOUT_PANIC=1")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()

	// 子进程应正常退出（exit code 0），不再因 panic 崩溃
	assert.NoError(t, err, "含 timeout 的链中 handler panic 应被 recover，子进程不应崩溃，stderr: %s",
		truncate(stderr.String(), 500))
	// 子进程应返回 500 状态码
	assert.Contains(t, stdout.String(), "STATUS: 500",
		"子进程应返回 500 状态码，实际输出: %s", truncate(stdout.String(), 500))
}

// truncate 截断字符串到指定长度，用于日志输出。
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
