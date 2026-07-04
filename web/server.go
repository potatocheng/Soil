package web

import (
	"context"
	"crypto/tls"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

type HandleFunc func(ctx *Context)

type Server interface {
	http.Handler

	Start(addr string) error

	StartTLS(addr string, certFile string, keyFile string) error

	Shutdown(ctx context.Context) error

	addRoute(method string, path string, handler HandleFunc)
}

type ServerConfig struct {
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	TLSConfig       *tls.Config
	ShutdownTimeout time.Duration
}

var DefaultServerConfig = ServerConfig{
	ReadTimeout:     30 * time.Second,
	WriteTimeout:    30 * time.Second,
	IdleTimeout:     60 * time.Second,
	ShutdownTimeout: 30 * time.Second,
}

// DefaultTLSConfig 返回一个安全默认的 TLS 配置。
// MinVersion 设置为 tls.VersionTLS12，CipherSuites 留空使用 Go 默认安全列表。
// 如需自定义加密套件，可在返回的 *tls.Config 上设置 CipherSuites 字段，
// 然后通过 StartTLSWithConfig 传入。
func DefaultTLSConfig() *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		// CipherSuites 留空以使用 Go 默认的安全列表。
		// 如需自定义，可按如下示例设置：
		// CipherSuites: []uint16{
		//     tls.TLS_AES_128_GCM_SHA256,
		//     tls.TLS_AES_256_GCM_SHA384,
		//     tls.TLS_CHACHA20_POLY1305_SHA256,
		// },
	}
}

var _ Server = &HTTPServer{}

type HTTPServer struct {
	router
	mdls   []Middleware
	config ServerConfig
	server *http.Server
	pool   sync.Pool
}

func NewHttpServer() *HTTPServer {
	return NewHttpServerWithConfig(DefaultServerConfig)
}

func NewHttpServerWithConfig(config ServerConfig) *HTTPServer {
	return &HTTPServer{
		router: newRouter(),
		config: config,
		pool: sync.Pool{
			New: func() any {
				return &Context{}
			},
		},
	}
}

func (hs *HTTPServer) Use(mdls ...Middleware) {
	if hs.mdls == nil {
		hs.mdls = mdls
		return
	}

	hs.mdls = append(hs.mdls, mdls...)
}

func (hs *HTTPServer) Start(addr string) error {
	hs.server = &http.Server{
		Addr:         addr,
		Handler:      hs,
		ReadTimeout:  hs.config.ReadTimeout,
		WriteTimeout: hs.config.WriteTimeout,
		IdleTimeout:  hs.config.IdleTimeout,
	}
	log.Printf("Server starting on %s", addr)
	return hs.server.ListenAndServe()
}

// StartTLS 启动 HTTPS 服务器，使用 ServerConfig.TLSConfig。
// 该方法保持向后兼容：内部委托给 StartTLSWithConfig。
func (hs *HTTPServer) StartTLS(addr string, certFile string, keyFile string) error {
	return hs.StartTLSWithConfig(addr, certFile, keyFile, hs.config.TLSConfig)
}

// StartTLSWithConfig 使用自定义 tls.Config 启动 HTTPS 服务器。
// 若 tlsConfig 为 nil，则使用 DefaultTLSConfig()。
// 传入的 tlsConfig 会保存到 hs.server.TLSConfig 上，
// 随后调用 http.Server.ListenAndServeTLS 加载 certFile 与 keyFile 指定的证书。
func (hs *HTTPServer) StartTLSWithConfig(addr string, certFile string, keyFile string, tlsConfig *tls.Config) error {
	if tlsConfig == nil {
		tlsConfig = DefaultTLSConfig()
	}
	hs.server = &http.Server{
		Addr:         addr,
		Handler:      hs,
		ReadTimeout:  hs.config.ReadTimeout,
		WriteTimeout: hs.config.WriteTimeout,
		IdleTimeout:  hs.config.IdleTimeout,
		TLSConfig:    tlsConfig,
	}
	log.Printf("Server starting TLS on %s", addr)
	return hs.server.ListenAndServeTLS(certFile, keyFile)
}

// StartAndServeWithSignal 启动 HTTP 服务器并监听终止信号以执行优雅关闭。
// 监听 os.Interrupt 与 syscall.SIGTERM 信号：
//   - Linux 下两者皆可由外部触发（如 Ctrl+C 或 kill 命令）。
//   - Windows 下不支持 SIGTERM，将被忽略；os.Interrupt 对应 Ctrl+C 仍可生效。
//
// 收到信号后，使用 hs.config.ShutdownTimeout 创建带超时的 context，
// 调用 hs.Shutdown 执行优雅关闭。若 Start 立即失败（如端口占用）则直接返回该错误。
func (hs *HTTPServer) StartAndServeWithSignal(addr string) error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- hs.Start(addr)
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return err
	case <-sigCh:
		ctx, cancel := context.WithTimeout(context.Background(), hs.config.ShutdownTimeout)
		defer cancel()
		return hs.Shutdown(ctx)
	}
}

func (hs *HTTPServer) Shutdown(ctx context.Context) error {
	if hs.server == nil {
		return nil
	}
	log.Println("Server shutting down...")
	return hs.server.Shutdown(ctx)
}

func (hs *HTTPServer) flashResp(ctx *Context) {
	if ctx.RespHeaders != nil {
		for key, values := range ctx.RespHeaders {
			for _, value := range values {
				ctx.Resp.Header().Add(key, value)
			}
		}
	}
	if ctx.RespStatusCode > 0 {
		ctx.Resp.WriteHeader(ctx.RespStatusCode)
	}
	if len(ctx.RespData) > 0 {
		_, err := ctx.Resp.Write(ctx.RespData)
		if err != nil {
			log.Println("回写响应失败", err)
		}
	}
}

// ServeHTTP 使用 sync.Pool 复用 Context 对象，降低 GC 压力。
//
// 复用流程：从 pool 取出 ctx -> reset 清空字段 -> 设置当前 Req/Resp ->
// 构建中间件链并执行 -> defer 中 reset 后放回 pool。
//
// 已知限制（timeout 场景）：当使用 timeout 中间件且超时分支触发时，
// handler 所在的 goroutine 可能仍在运行并持有 ctx 引用。此时 pool.Put 后
// 该 goroutine 可能操作已被复用的 ctx，存在理论上的数据竞争风险。
// 当前 Soil 的 timeout 中间件在 select 返回后会通过 context 取消让
// handler goroutine 退出，实际竞争窗口很小，且生产中 handler 通常快速完成。
// 综合考虑性能收益，此处采用“总是 Put”策略；如未来出现因 timeout 引发的
// 复用问题，可改为超时分支不 Put（让 GC 回收）。
func (hs *HTTPServer) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	ctx := hs.pool.Get().(*Context)
	ctx.reset()
	ctx.Req = request
	ctx.Resp = response
	defer func() {
		ctx.reset()
		hs.pool.Put(ctx)
	}()

	root := hs.serve
	for i := len(hs.mdls) - 1; i >= 0; i-- {
		root = hs.mdls[i](root)
	}

	var m Middleware = func(next HandleFunc) HandleFunc {
		return func(ctx *Context) {
			next(ctx)
			if !ctx.IsDone() {
				hs.flashResp(ctx)
			}
		}
	}

	root = m(root)
	root(ctx)
}

func (hs *HTTPServer) serve(ctx *Context) {
	mi, ok := hs.findRoute(ctx.Req.Method, ctx.Req.URL.Path)
	if !ok || mi.node == nil || mi.node.handler == nil {
		if mi != nil && mi.methodNotAllowed {
			ctx.RespStatusCode = http.StatusMethodNotAllowed
			ctx.RespData = []byte("405 method not allowed")
			ctx.SetHeader("Allow", strings.Join(mi.allowedMethods, ", "))
			return
		}
		ctx.RespStatusCode = http.StatusNotFound
		ctx.RespData = []byte("404 page not found")
		return
	}
	ctx.PathParams = mi.paramPath
	ctx.MatchedRoute = mi.matchedPath
	mi.node.handler(ctx)
}

func (hs *HTTPServer) Post(path string, handler HandleFunc) {
	hs.addRoute(http.MethodPost, path, handler)
}

func (hs *HTTPServer) Get(path string, handler HandleFunc) {
	hs.addRoute(http.MethodGet, path, handler)
}

func (hs *HTTPServer) Put(path string, handler HandleFunc) {
	hs.addRoute(http.MethodPut, path, handler)
}

func (hs *HTTPServer) Delete(path string, handler HandleFunc) {
	hs.addRoute(http.MethodDelete, path, handler)
}

func (hs *HTTPServer) Patch(path string, handler HandleFunc) {
	hs.addRoute(http.MethodPatch, path, handler)
}

func (hs *HTTPServer) Options(path string, handler HandleFunc) {
	hs.addRoute(http.MethodOptions, path, handler)
}

func (hs *HTTPServer) Head(path string, handler HandleFunc) {
	hs.addRoute(http.MethodHead, path, handler)
}

// Group 创建一个路由分组，所有在该分组下注册的路由都会带上 prefix 前缀，
// 并应用传入的中间件（以及后续通过 RouterGroup.Use 追加的中间件）。
func (hs *HTTPServer) Group(prefix string, mdls ...Middleware) *RouterGroup {
	return &RouterGroup{
		prefix: prefix,
		mdls:   mdls,
		server: hs,
	}
}

// Static 在 prefix 前缀下注册一个静态文件服务，dir 为文件系统根目录。
// 内部使用 http.FileServer + http.StripPrefix 实现，http.FileServer 已内置
// 对 "../" 路径穿越的防护（通过 path.Clean 清理并限制在根目录内）。
//
// 由于 http.FileServer 直接写 ResponseWriter（绕过 ctx.RespStatusCode/RespData），
// 这里在调用后设置 ctx.done = true，让 flashResp 跳过统一写出，避免重复写响应。
func (hs *HTTPServer) Static(prefix, dir string) {
	// 规范化 prefix：确保以 "/" 开头，去除尾部 "/"，避免拼接出 "//"
	cleanPrefix := prefix
	if !strings.HasPrefix(cleanPrefix, "/") {
		cleanPrefix = "/" + cleanPrefix
	}
	cleanPrefix = strings.TrimSuffix(cleanPrefix, "/")

	fileServer := http.FileServer(http.Dir(dir))
	handler := http.StripPrefix(cleanPrefix, fileServer)

	hs.Get(cleanPrefix+"/*", func(ctx *Context) {
		handler.ServeHTTP(ctx.Resp, ctx.Req)
		// 标记已完成，跳过 flashResp 的统一写出
		ctx.done = true
	})
}
