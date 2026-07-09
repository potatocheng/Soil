package net

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// Client 是生产级 TCP 客户端（多路复用 + 多 endpoint 负载均衡）
type Client struct {
	network string
	// address 保留创建时原始串，便于日志
	address string
	opts    *clientOptions

	nodes    []*endpointNode
	balancer Balancer
	nodesMu  sync.RWMutex // 保护 Resolve 刷新时替换 nodes

	closed atomic.Bool
	reqID  atomic.Uint64
}

// NewClient 创建客户端。
// address 支持单地址或多地址：
//
//	"127.0.0.1:8080"
//	"127.0.0.1:8080,127.0.0.1:8081"
//	"127.0.0.1:8080@3,127.0.0.1:8081@1"  // 带权重
//
// 也可配合 WithEndpoints / WithEndpointList / WithDNSResolve。
func NewClient(network, address string, opts ...ClientOption) *Client {
	o := defaultClientOptions()
	for _, opt := range opts {
		opt(o)
	}

	c := &Client{
		network:  network,
		address:  address,
		opts:     o,
		balancer: o.balancer,
	}
	if c.balancer == nil {
		c.balancer = NewBalancer(PolicyRoundRobin)
	}

	eps, err := c.buildEndpoints(context.Background(), address, o)
	if err != nil {
		// 延迟到首次 Call 失败更难排查；这里用单节点占位并在 Call 时再报错不理想
		// 直接 panic 不友好；保留解析错误到 nodes 为空，Call 返回 err
		c.opts.logger.Error("parse endpoints failed", "error", err, "address", address)
		// 仍尝试把原始 address 当作单 endpoint（兼容旧用法）
		if ep, e2 := parseOneEndpoint(address); e2 == nil {
			eps = []Endpoint{ep}
		}
	}

	c.nodes = c.newNodes(eps)
	return c
}

func (c *Client) buildEndpoints(ctx context.Context, address string, o *clientOptions) ([]Endpoint, error) {
	var eps []Endpoint
	if address != "" {
		parsed, err := parseEndpointList(address)
		if err != nil {
			return nil, err
		}
		eps = parsed
	}
	eps = mergeEndpoints(eps, o.extraEndpoints)
	if len(eps) == 0 {
		return nil, errors.New("client: no endpoints configured")
	}

	if o.dnsResolve {
		resolver := o.resolver
		if resolver == nil {
			resolver = &DNSResolver{}
		}
		resolved, err := resolveEndpoints(ctx, c.network, eps, resolver)
		if err != nil {
			return nil, err
		}
		eps = resolved
	}
	return eps, nil
}

func (c *Client) newNodes(eps []Endpoint) []*endpointNode {
	nodes := make([]*endpointNode, 0, len(eps))
	for _, ep := range eps {
		n := &endpointNode{
			ep:   ep,
			pool: newStreamPool(c, ep.Addr),
		}
		nodes = append(nodes, n)
	}
	return nodes
}

func (c *Client) dialAddr(address string) (net.Conn, error) {
	var d net.Dialer
	d.Timeout = c.opts.dialTimeout
	d.KeepAlive = c.opts.keepAlive

	var conn net.Conn
	var err error
	if c.opts.tlsConfig != nil {
		conn, err = tls.DialWithDialer(&d, c.network, address, c.opts.tlsConfig)
	} else {
		conn, err = d.Dial(c.network, address)
	}
	if err != nil {
		return nil, err
	}
	c.opts.metrics.ConnectionOpened("client")
	return conn, nil
}

// Call 发送请求并等待响应。
// 多 endpoint 时按 Balancer 选路；传输失败会标记节点并在重试时换路。
func (c *Client) Call(ctx context.Context, req *Request) (*Response, error) {
	if c.closed.Load() {
		return nil, errPoolClosed
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if c.opts.requestTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.opts.requestTimeout)
		defer cancel()
	}

	if req.RequestID == 0 {
		req.RequestID = c.nextRequestID()
	}

	var lastErr error
	// 重试次数：用户配置 + 至少尝试所有健康节点一次的机会由 attempts 覆盖
	maxAttempts := c.opts.retryAttempts
	if maxAttempts < 0 {
		maxAttempts = 0
	}

	for attempt := 0; attempt <= maxAttempts; attempt++ {
		start := time.Now()
		resp, err := c.doCall(ctx, req)
		c.opts.metrics.RequestSent(time.Since(start), err)
		if err == nil {
			return resp, nil
		}
		if errors.Is(err, ErrRemote) || errors.Is(err, ErrRateLimited) || errors.Is(err, ErrHandlerPanic) {
			return nil, err
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		lastErr = err
		c.opts.logger.Warn("call failed", "attempt", attempt, "error", err)
		if attempt < maxAttempts {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(c.opts.retryBackoff):
			}
		}
	}
	return nil, lastErr
}

func (c *Client) doCall(ctx context.Context, req *Request) (*Response, error) {
	node, err := c.pickNode(ctx)
	if err != nil {
		return nil, err
	}
	// 选路后立即预占，让 least_active 看见即将发生的负载
	node.reserve()
	defer node.unreserve()

	s, err := node.pool.Get(ctx)
	if err != nil {
		node.MarkFailure(c.opts.healthThreshold, c.opts.healthCooldown)
		return nil, err
	}

	resp, err := s.Call(ctx, req)
	if err != nil {
		// 业务错误不算节点故障；传输/连接错误才记失败
		if errors.Is(err, ErrRemote) || errors.Is(err, ErrRateLimited) || errors.Is(err, ErrHandlerPanic) {
			node.MarkSuccess()
			return nil, err
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			// 超时不一定是节点坏；不记成功也不大力记失败
			return nil, err
		}
		node.MarkFailure(c.opts.healthThreshold, c.opts.healthCooldown)
		return nil, err
	}
	node.MarkSuccess()
	return resp, nil
}

func (c *Client) pickNode(ctx context.Context) (*endpointNode, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	c.nodesMu.RLock()
	nodes := c.nodes
	c.nodesMu.RUnlock()

	if len(nodes) == 0 {
		return nil, ErrNoEndpoint
	}

	// 优先健康节点
	healthy := make([]*endpointNode, 0, len(nodes))
	for _, n := range nodes {
		if n.Healthy() {
			healthy = append(healthy, n)
		}
	}
	candidates := healthy
	if len(candidates) == 0 {
		// 全部熔断时降级：仍尝试全部，避免雪崩后永久不可用
		candidates = nodes
	}

	n := c.balancer.Pick(candidates)
	if n == nil {
		return nil, ErrNoEndpoint
	}
	return n, nil
}

// Resolve 重新解析 endpoint（例如 DNS 变更后手动刷新）。
// 会关闭旧 pool 并替换节点列表。
func (c *Client) Resolve(ctx context.Context) error {
	if c.closed.Load() {
		return errPoolClosed
	}
	eps, err := c.buildEndpoints(ctx, c.address, c.opts)
	if err != nil {
		return err
	}

	newNodes := c.newNodes(eps)

	c.nodesMu.Lock()
	old := c.nodes
	c.nodes = newNodes
	c.nodesMu.Unlock()

	for _, n := range old {
		_ = n.pool.Close()
	}
	return nil
}

// Endpoints 返回当前（解析后）地址列表
func (c *Client) Endpoints() []string {
	c.nodesMu.RLock()
	defer c.nodesMu.RUnlock()
	out := make([]string, len(c.nodes))
	for i, n := range c.nodes {
		out[i] = n.Addr()
	}
	return out
}

// EndpointStats 返回各 endpoint 状态
func (c *Client) EndpointStats() []EndpointStats {
	c.nodesMu.RLock()
	defer c.nodesMu.RUnlock()
	out := make([]EndpointStats, len(c.nodes))
	for i, n := range c.nodes {
		out[i] = n.Stats()
	}
	return out
}

// Stats 聚合所有 endpoint 的连接池统计
func (c *Client) Stats() PoolStats {
	c.nodesMu.RLock()
	defer c.nodesMu.RUnlock()
	var st PoolStats
	for _, n := range c.nodes {
		ps := n.pool.Stats()
		st.Open += ps.Open
		st.DialCount += ps.DialCount
		st.ClosedIdle += ps.ClosedIdle
		st.ClosedLife += ps.ClosedLife
		st.ClosedBad += ps.ClosedBad
		st.HeartbeatOK += ps.HeartbeatOK
		st.HeartbeatErr += ps.HeartbeatErr
		st.TotalInflight += ps.TotalInflight
	}
	return st
}

// Communicate 兼容旧 API 的高级别调用
func (c *Client) Communicate(data string) (string, error) {
	resp, err := c.Call(context.Background(), &Request{Body: []byte(data)})
	if err != nil {
		return "", err
	}
	if resp.Error != nil {
		return "", resp.Error
	}
	return string(resp.Body), nil
}

// Send 兼容旧 API：单向发送
func (c *Client) Send(data string) error {
	_, err := c.Call(context.Background(), &Request{
		Body:   []byte(data),
		OneWay: true,
	})
	return err
}

// Receive 兼容旧 API：已弃用
func (c *Client) Receive() (string, error) {
	return "", errors.New("Receive is deprecated, use Call instead")
}

func (c *Client) nextRequestID() uint64 {
	return c.reqID.Add(1)
}

// Close 关闭客户端及其全部 stream
func (c *Client) Close() error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	c.nodesMu.Lock()
	nodes := c.nodes
	c.nodes = nil
	c.nodesMu.Unlock()

	var firstErr error
	for _, n := range nodes {
		if err := n.pool.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// String 便于日志
func (c *Client) String() string {
	return fmt.Sprintf("Client(%s://%s endpoints=%v)", c.network, c.address, c.Endpoints())
}
