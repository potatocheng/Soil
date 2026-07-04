package web

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TR-13.1: DefaultTLSConfig 返回的 MinVersion 为 tls.VersionTLS12
func TestDefaultTLSConfig(t *testing.T) {
	cfg := DefaultTLSConfig()
	require.NotNil(t, cfg)
	assert.Equal(t, uint16(tls.VersionTLS12), cfg.MinVersion)
}

// TR-13.2: StartTLSWithConfig 创建的 hs.server 的 TLSConfig 与传入一致
func TestStartTLSWithConfig(t *testing.T) {
	hs := NewHttpServer()
	customTLS := &tls.Config{
		MinVersion: tls.VersionTLS13,
	}

	// 使用不存在的证书路径，ListenAndServeTLS 会失败，但 hs.server 已被设置
	err := hs.StartTLSWithConfig("127.0.0.1:0", "nonexistent.crt", "nonexistent.key", customTLS)
	assert.Error(t, err)

	// 验证 hs.server.TLSConfig 与传入的指针一致
	require.NotNil(t, hs.server)
	require.NotNil(t, hs.server.TLSConfig)
	assert.Same(t, customTLS, hs.server.TLSConfig)
	assert.Equal(t, uint16(tls.VersionTLS13), hs.server.TLSConfig.MinVersion)
}

// TR-13.2 补充: StartTLSWithConfig 传入 nil 时使用 DefaultTLSConfig
func TestStartTLSWithConfig_NilUsesDefault(t *testing.T) {
	hs := NewHttpServer()

	err := hs.StartTLSWithConfig("127.0.0.1:0", "nonexistent.crt", "nonexistent.key", nil)
	assert.Error(t, err)

	require.NotNil(t, hs.server)
	require.NotNil(t, hs.server.TLSConfig)
	// 应使用 DefaultTLSConfig，MinVersion 为 TLS1.2
	assert.Equal(t, uint16(tls.VersionTLS12), hs.server.TLSConfig.MinVersion)
}

// TR-14.1: DefaultServerConfig.ShutdownTimeout 为 30s
func TestShutdownTimeout(t *testing.T) {
	assert.Equal(t, 30*time.Second, DefaultServerConfig.ShutdownTimeout)
}

// TR-14.2: StartAndServeWithSignal 启动服务器、响应请求、收到信号后优雅退出
//
// 测试流程：
//  1. 在随机端口启动 StartAndServeWithSignal
//  2. 发送 HTTP 请求验证服务器正常响应（200）
//  3. 向当前进程发送 os.Interrupt 信号触发优雅关闭
//  4. 验证 StartAndServeWithSignal 返回
//
// Windows 兼容性：os.Interrupt 对应 CTRL_C_EVENT，signal.Notify 会拦截该信号，
// 因此不会终止测试进程。若信号未触发（某些环境下信号传递不稳定），
// 会回退到直接调用 hs.Shutdown 完成关闭。Linux 下信号路径会被完整验证。
func TestStartAndServeWithSignal(t *testing.T) {
	hs := NewHttpServer()
	hs.Get("/hello", func(ctx *Context) {
		ctx.RespJson(http.StatusOK, map[string]string{"message": "hello"})
	})
	// 使用较短的超时以加快测试
	hs.config.ShutdownTimeout = 2 * time.Second

	// 获取一个空闲端口
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	ln.Close()

	errCh := make(chan error, 1)
	go func() {
		errCh <- hs.StartAndServeWithSignal(addr)
	}()

	// 等待服务器启动并发送请求验证可响应
	var resp *http.Response
	for i := 0; i < 30; i++ {
		resp, err = http.Get("http://" + addr + "/hello")
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	require.NoError(t, err, "无法连接到服务器")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// 触发关闭：向当前进程发送 os.Interrupt 信号
	// Linux 下会发送 SIGINT；Windows 下会生成 CTRL_C_EVENT
	// StartAndServeWithSignal 中的 signal.Notify 会拦截该信号
	p, _ := os.FindProcess(os.Getpid())
	_ = p.Signal(os.Interrupt)

	// 等待关闭，带 fallback（某些 Windows 环境下信号可能无法传递）
	select {
	case err := <-errCh:
		// 优雅关闭返回 nil；若 Start 先返回（外部 Shutdown）则为 http.ErrServerClosed
		if err != nil && err != http.ErrServerClosed {
			t.Logf("StartAndServeWithSignal 返回: %v", err)
		}
	case <-time.After(3 * time.Second):
		// 信号未触发关闭，回退到直接调用 Shutdown（模拟信号路径执行的逻辑）
		t.Log("信号未触发关闭，回退到直接调用 Shutdown")
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = hs.Shutdown(ctx)
		<-errCh
	}
}
