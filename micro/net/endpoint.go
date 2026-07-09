package net

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// Endpoint 表示一个后端地址（可带权重）
type Endpoint struct {
	// Addr 形式为 host:port，或已解析的 ip:port
	Addr string
	// Weight 权重，<=0 时按 1 处理
	Weight int
}

// EndpointStats 单个 endpoint 的运行快照
type EndpointStats struct {
	Addr      string
	Weight    int
	Healthy   bool
	Fails     int32
	DownUntil time.Time
	Pool      PoolStats
}

// Resolver 将逻辑地址解析为可拨号的 host:port 列表
type Resolver interface {
	Resolve(ctx context.Context, network, address string) ([]string, error)
}

// DNSResolver 通过 DNS A/AAAA 将 hostname:port 展开为多个 ip:port
type DNSResolver struct {
	// Net 可选自定义 *net.Resolver；nil 使用系统默认
	Net *net.Resolver
}

// Resolve 实现 Resolver
func (r *DNSResolver) Resolve(ctx context.Context, network, address string) ([]string, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("endpoint: invalid address %q: %w", address, err)
	}
	// 已是 IP 则无需解析
	if ip := net.ParseIP(host); ip != nil {
		return []string{address}, nil
	}

	resolver := r.Net
	if resolver == nil {
		resolver = net.DefaultResolver
	}

	ips, err := resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("endpoint: resolve %q: %w", host, err)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("endpoint: no addresses for %q", host)
	}

	out := make([]string, 0, len(ips))
	seen := make(map[string]struct{}, len(ips))
	for _, ipa := range ips {
		ip := ipa.IP
		// 按 network 偏好过滤（tcp4/tcp6）；tcp 则全收
		if network == "tcp4" && ip.To4() == nil {
			continue
		}
		if network == "tcp6" && ip.To4() != nil {
			continue
		}
		addr := net.JoinHostPort(ip.String(), port)
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
		out = append(out, addr)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("endpoint: no usable addresses for %q on %s", host, network)
	}
	return out, nil
}

// StaticResolver 原样返回，不做 DNS
type StaticResolver struct{}

func (StaticResolver) Resolve(_ context.Context, _, address string) ([]string, error) {
	if _, _, err := net.SplitHostPort(address); err != nil {
		return nil, fmt.Errorf("endpoint: invalid address %q: %w", address, err)
	}
	return []string{address}, nil
}

// parseEndpointList 解析多地址字符串。
// 支持分隔符: 逗号、分号、空白。
// 单条目支持权重后缀: host:port@weight
// 示例: "a:1,b:2@3; c:3"
func parseEndpointList(raw string) ([]Endpoint, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("endpoint: empty address")
	}
	// 统一分隔
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n'
	})
	if len(fields) == 0 {
		return nil, errors.New("endpoint: empty address")
	}

	eps := make([]Endpoint, 0, len(fields))
	for _, f := range fields {
		ep, err := parseOneEndpoint(f)
		if err != nil {
			return nil, err
		}
		eps = append(eps, ep)
	}
	return dedupeEndpoints(eps), nil
}

func parseOneEndpoint(s string) (Endpoint, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return Endpoint{}, errors.New("endpoint: empty entry")
	}
	weight := 1
	addr := s
	if i := strings.LastIndex(s, "@"); i >= 0 {
		addr = s[:i]
		w, err := strconv.Atoi(s[i+1:])
		if err != nil || w <= 0 {
			return Endpoint{}, fmt.Errorf("endpoint: invalid weight in %q", s)
		}
		weight = w
	}
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return Endpoint{}, fmt.Errorf("endpoint: invalid address %q: %w", addr, err)
	}
	return Endpoint{Addr: addr, Weight: weight}, nil
}

func dedupeEndpoints(in []Endpoint) []Endpoint {
	seen := make(map[string]int, len(in)) // addr -> index
	out := make([]Endpoint, 0, len(in))
	for _, ep := range in {
		if ep.Weight <= 0 {
			ep.Weight = 1
		}
		if idx, ok := seen[ep.Addr]; ok {
			// 同地址保留较大权重
			if ep.Weight > out[idx].Weight {
				out[idx].Weight = ep.Weight
			}
			continue
		}
		seen[ep.Addr] = len(out)
		out = append(out, ep)
	}
	return out
}

func mergeEndpoints(base []Endpoint, extra []Endpoint) []Endpoint {
	return dedupeEndpoints(append(append([]Endpoint{}, base...), extra...))
}

// resolveEndpoints 对每个逻辑 endpoint 做 Resolver 展开，权重继承到每个解析结果
func resolveEndpoints(ctx context.Context, network string, eps []Endpoint, resolver Resolver) ([]Endpoint, error) {
	if resolver == nil {
		resolver = StaticResolver{}
	}
	out := make([]Endpoint, 0, len(eps))
	for _, ep := range eps {
		addrs, err := resolver.Resolve(ctx, network, ep.Addr)
		if err != nil {
			return nil, err
		}
		for _, a := range addrs {
			out = append(out, Endpoint{Addr: a, Weight: ep.Weight})
		}
	}
	if len(out) == 0 {
		return nil, errors.New("endpoint: no resolved addresses")
	}
	return dedupeEndpoints(out), nil
}

// endpointNode 运行时节点：独立 stream 池 + 健康状态
type endpointNode struct {
	ep   Endpoint
	pool *streamPool

	fails     atomic.Int32
	downUntil atomic.Int64 // unix nano；0 表示健康
	// reserved 为选路后、stream.Call 完成前的预占，避免 least-active 窗口失真
	reserved atomic.Int32
}

func (n *endpointNode) Addr() string { return n.ep.Addr }

func (n *endpointNode) Weight() int {
	if n.ep.Weight <= 0 {
		return 1
	}
	return n.ep.Weight
}

func (n *endpointNode) Healthy() bool {
	until := n.downUntil.Load()
	if until == 0 {
		return true
	}
	return time.Now().UnixNano() >= until
}

func (n *endpointNode) MarkSuccess() {
	n.fails.Store(0)
	n.downUntil.Store(0)
}

func (n *endpointNode) MarkFailure(threshold int, cooldown time.Duration) {
	if threshold <= 0 {
		threshold = 2
	}
	if cooldown <= 0 {
		cooldown = 5 * time.Second
	}
	f := n.fails.Add(1)
	if int(f) >= threshold {
		n.downUntil.Store(time.Now().Add(cooldown).UnixNano())
	}
}

func (n *endpointNode) Stats() EndpointStats {
	until := n.downUntil.Load()
	var t time.Time
	if until > 0 {
		t = time.Unix(0, until)
	}
	return EndpointStats{
		Addr:      n.ep.Addr,
		Weight:    n.Weight(),
		Healthy:   n.Healthy(),
		Fails:     n.fails.Load(),
		DownUntil: t,
		Pool:      n.pool.Stats(),
	}
}

// Inflight 当前在途请求数（预占 + 池内 in-flight，用于 least-active）
func (n *endpointNode) Inflight() int32 {
	return n.reserved.Load() + n.pool.Stats().TotalInflight
}

func (n *endpointNode) reserve()   { n.reserved.Add(1) }
func (n *endpointNode) unreserve() { n.reserved.Add(-1) }
