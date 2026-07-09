package net

import (
	"crypto/tls"
	"time"
)

// clientOptions 保存客户端配置
type clientOptions struct {
	dialTimeout    time.Duration
	readTimeout    time.Duration
	writeTimeout   time.Duration
	requestTimeout time.Duration
	keepAlive      time.Duration
	idleTimeout    time.Duration
	maxConnLifetime time.Duration
	maxHeaderSize  uint32
	maxBodySize    uint32
	maxIdleConns   int // 兼容保留；多路复用下以 maxOpen 为主
	maxOpenConns   int
	retryAttempts  int
	retryBackoff   time.Duration
	// 应用层心跳：空闲超过 heartbeatInterval 时发 Ping
	heartbeatInterval time.Duration
	heartbeatTimeout  time.Duration
	logger            Logger
	metrics           Metrics
	tlsConfig         *tls.Config

	// 多 endpoint / 负载均衡
	extraEndpoints  []Endpoint
	balancer        Balancer
	dnsResolve      bool
	resolver        Resolver
	healthThreshold int           // 连续失败次数触发熔断
	healthCooldown  time.Duration // 熔断冷却时间
}

// ClientOption 配置客户端
type ClientOption func(*clientOptions)

func defaultClientOptions() *clientOptions {
	return &clientOptions{
		dialTimeout:       3 * time.Second,
		readTimeout:       5 * time.Second,
		writeTimeout:      5 * time.Second,
		requestTimeout:    10 * time.Second,
		keepAlive:         30 * time.Second,
		idleTimeout:       60 * time.Second,
		maxConnLifetime:    0, // 0 表示不限制
		maxHeaderSize:     defaultMaxHeaderSize,
		maxBodySize:       defaultMaxBodySize,
		maxIdleConns:      10,
		maxOpenConns:      10,
		retryAttempts:     0,
		retryBackoff:      100 * time.Millisecond,
		heartbeatInterval: 30 * time.Second,
		heartbeatTimeout:  3 * time.Second,
		logger:            defaultLogger,
		metrics:           defaultMetrics,
		balancer:          NewBalancer(PolicyRoundRobin),
		healthThreshold:   2,
		healthCooldown:    5 * time.Second,
	}
}

func (o *clientOptions) limits() frameLimits {
	return frameLimits{maxHeader: o.maxHeaderSize, maxBody: o.maxBodySize}
}

// WithDialTimeout 设置拨号超时
func WithDialTimeout(d time.Duration) ClientOption {
	return func(o *clientOptions) { o.dialTimeout = d }
}

// WithReadTimeout 设置读超时
func WithReadTimeout(d time.Duration) ClientOption {
	return func(o *clientOptions) { o.readTimeout = d }
}

// WithWriteTimeout 设置写超时
func WithWriteTimeout(d time.Duration) ClientOption {
	return func(o *clientOptions) { o.writeTimeout = d }
}

// WithRequestTimeout 设置单次请求超时（会与 ctx deadline 取更早者）
func WithRequestTimeout(d time.Duration) ClientOption {
	return func(o *clientOptions) { o.requestTimeout = d }
}

// WithKeepAlive 设置 TCP keepalive 间隔
func WithKeepAlive(d time.Duration) ClientOption {
	return func(o *clientOptions) { o.keepAlive = d }
}

// WithIdleTimeout 设置连接池空闲超时
func WithIdleTimeout(d time.Duration) ClientOption {
	return func(o *clientOptions) { o.idleTimeout = d }
}

// WithMaxConns 设置连接池空闲与打开上限（二者相同，兼容旧 API）
func WithMaxConns(n int) ClientOption {
	return func(o *clientOptions) {
		o.maxIdleConns = n
		o.maxOpenConns = n
	}
}

// WithMaxIdleConns 设置空闲连接上限
func WithMaxIdleConns(n int) ClientOption {
	return func(o *clientOptions) { o.maxIdleConns = n }
}

// WithMaxOpenConns 设置最大多路复用 stream 数
func WithMaxOpenConns(n int) ClientOption {
	return func(o *clientOptions) { o.maxOpenConns = n }
}

// WithMaxConnLifetime 设置单条 stream 最大存活时间（空闲时回收）
func WithMaxConnLifetime(d time.Duration) ClientOption {
	return func(o *clientOptions) { o.maxConnLifetime = d }
}

// WithHeartbeat 设置应用层心跳间隔与超时（interval<=0 关闭心跳）
func WithHeartbeat(interval, timeout time.Duration) ClientOption {
	return func(o *clientOptions) {
		o.heartbeatInterval = interval
		o.heartbeatTimeout = timeout
	}
}

// WithMaxMessageSize 设置单帧 Header/Body 上限
func WithMaxMessageSize(maxHeader, maxBody uint32) ClientOption {
	return func(o *clientOptions) {
		if maxHeader > 0 {
			o.maxHeaderSize = maxHeader
		}
		if maxBody > 0 {
			o.maxBodySize = maxBody
		}
	}
}

// WithRetry 设置失败重试次数与退避时间
func WithRetry(attempts int, backoff time.Duration) ClientOption {
	return func(o *clientOptions) {
		o.retryAttempts = attempts
		o.retryBackoff = backoff
	}
}

// WithLogger 设置日志器
func WithLogger(l Logger) ClientOption {
	return func(o *clientOptions) { o.logger = l }
}

// WithMetrics 设置指标收集器
func WithMetrics(m Metrics) ClientOption {
	return func(o *clientOptions) { o.metrics = m }
}

// WithTLSConfig 设置 TLS 配置
func WithTLSConfig(cfg *tls.Config) ClientOption {
	return func(o *clientOptions) { o.tlsConfig = cfg }
}

// WithEndpoints 追加后端地址（host:port 或 host:port@weight）
func WithEndpoints(addrs ...string) ClientOption {
	return func(o *clientOptions) {
		for _, a := range addrs {
			ep, err := parseOneEndpoint(a)
			if err != nil {
				continue
			}
			o.extraEndpoints = append(o.extraEndpoints, ep)
		}
	}
}

// WithEndpointList 追加结构化 endpoint 列表
func WithEndpointList(eps ...Endpoint) ClientOption {
	return func(o *clientOptions) {
		o.extraEndpoints = append(o.extraEndpoints, eps...)
	}
}

// WithBalancer 设置自定义负载均衡器
func WithBalancer(b Balancer) ClientOption {
	return func(o *clientOptions) {
		if b != nil {
			o.balancer = b
		}
	}
}

// WithBalancerPolicy 使用内置策略：round_robin / random / least_active / weighted_round_robin
func WithBalancerPolicy(policy BalancerPolicy) ClientOption {
	return func(o *clientOptions) {
		o.balancer = NewBalancer(policy)
	}
}

// WithDNSResolve 启用 DNS 解析：将 hostname:port 展开为全部 A/AAAA 记录
func WithDNSResolve(enable bool) ClientOption {
	return func(o *clientOptions) { o.dnsResolve = enable }
}

// WithResolver 自定义地址解析器（默认在 WithDNSResolve(true) 时用 DNSResolver）
func WithResolver(r Resolver) ClientOption {
	return func(o *clientOptions) { o.resolver = r }
}

// WithEndpointHealth 设置 endpoint 熔断：连续失败 threshold 次后冷却 cooldown
func WithEndpointHealth(threshold int, cooldown time.Duration) ClientOption {
	return func(o *clientOptions) {
		if threshold > 0 {
			o.healthThreshold = threshold
		}
		if cooldown > 0 {
			o.healthCooldown = cooldown
		}
	}
}

// serverOptions 保存服务端配置
type serverOptions struct {
	readTimeout    time.Duration
	writeTimeout   time.Duration
	idleTimeout    time.Duration
	maxConns       int
	maxHeaderSize  uint32
	maxBodySize    uint32
	handler        Handler
	middlewares    []Middleware
	logger         Logger
	metrics        Metrics
	tlsConfig      *tls.Config
	limiter        RateLimiter
	handlerTimeout time.Duration
}

// ServerOption 配置服务端
type ServerOption func(*serverOptions)

func defaultServerOptions() *serverOptions {
	return &serverOptions{
		readTimeout:    5 * time.Second,
		writeTimeout:   5 * time.Second,
		idleTimeout:    120 * time.Second,
		maxConns:       1000,
		maxHeaderSize:  defaultMaxHeaderSize,
		maxBodySize:    defaultMaxBodySize,
		handler:        EchoHandler,
		logger:         defaultLogger,
		metrics:        defaultMetrics,
		limiter:        defaultRateLimiter,
	}
}

func (o *serverOptions) limits() frameLimits {
	return frameLimits{maxHeader: o.maxHeaderSize, maxBody: o.maxBodySize}
}

func (o *serverOptions) buildHandler() Handler {
	if len(o.middlewares) == 0 {
		return o.handler
	}
	return Chain(o.handler, o.middlewares...)
}

// WithServerReadTimeout 设置单次读帧超时（与 idle 叠加时取更短）
func WithServerReadTimeout(d time.Duration) ServerOption {
	return func(o *serverOptions) { o.readTimeout = d }
}

// WithServerWriteTimeout 设置服务端写超时
func WithServerWriteTimeout(d time.Duration) ServerOption {
	return func(o *serverOptions) { o.writeTimeout = d }
}

// WithServerIdleTimeout 设置连接空闲超时
func WithServerIdleTimeout(d time.Duration) ServerOption {
	return func(o *serverOptions) { o.idleTimeout = d }
}

// WithMaxConnections 设置最大并发连接数
func WithMaxConnections(n int) ServerOption {
	return func(o *serverOptions) { o.maxConns = n }
}

// WithHandler 设置请求处理器
func WithHandler(h Handler) ServerOption {
	return func(o *serverOptions) { o.handler = h }
}

// WithMiddleware 追加服务端中间件（按调用顺序从外到内）
func WithMiddleware(mws ...Middleware) ServerOption {
	return func(o *serverOptions) {
		o.middlewares = append(o.middlewares, mws...)
	}
}

// WithHandlerTimeout 设置单个 Handler 的处理超时
func WithHandlerTimeout(d time.Duration) ServerOption {
	return func(o *serverOptions) { o.handlerTimeout = d }
}

// WithServerMaxMessageSize 设置服务端单帧 Header/Body 上限
func WithServerMaxMessageSize(maxHeader, maxBody uint32) ServerOption {
	return func(o *serverOptions) {
		if maxHeader > 0 {
			o.maxHeaderSize = maxHeader
		}
		if maxBody > 0 {
			o.maxBodySize = maxBody
		}
	}
}

// WithServerLogger 设置服务端日志器
func WithServerLogger(l Logger) ServerOption {
	return func(o *serverOptions) { o.logger = l }
}

// WithServerMetrics 设置服务端指标收集器
func WithServerMetrics(m Metrics) ServerOption {
	return func(o *serverOptions) { o.metrics = m }
}

// WithServerTLSConfig 设置服务端 TLS 配置
func WithServerTLSConfig(cfg *tls.Config) ServerOption {
	return func(o *serverOptions) { o.tlsConfig = cfg }
}

// WithRateLimiter 设置服务端限流器
func WithRateLimiter(l RateLimiter) ServerOption {
	return func(o *serverOptions) { o.limiter = l }
}
