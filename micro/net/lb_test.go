package net

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func startTaggedServer(t *testing.T, addr, tag string) *Server {
	t.Helper()
	handler := HandlerFunc(func(ctx context.Context, req *Request) (*Response, error) {
		return &Response{Body: []byte(tag)}, nil
	})
	serv := NewServer("tcp", addr, WithHandler(handler))
	go func() { _ = serv.Start() }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = serv.Shutdown(ctx)
	})
	waitListen(t, "127.0.0.1"+addr)
	return serv
}

func waitListen(t *testing.T, hostport string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", hostport, 50*time.Millisecond)
		if err == nil {
			_ = c.Close()
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("server not listening on %s", hostport)
}

func TestClientMultiEndpointRoundRobin(t *testing.T) {
	_ = startTaggedServer(t, ":19101", "A")
	_ = startTaggedServer(t, ":19102", "B")

	cli := NewClient("tcp", "127.0.0.1:19101,127.0.0.1:19102",
		WithHeartbeat(0, 0),
		WithBalancerPolicy(PolicyRoundRobin),
		WithMaxOpenConns(2),
	)
	defer func() { _ = cli.Close() }()

	require.ElementsMatch(t, []string{"127.0.0.1:19101", "127.0.0.1:19102"}, cli.Endpoints())

	got := make(map[string]int)
	for i := 0; i < 20; i++ {
		resp, err := cli.Call(context.Background(), &Request{Body: []byte("x")})
		require.NoError(t, err)
		got[string(resp.Body)]++
	}
	assert.Equal(t, 10, got["A"])
	assert.Equal(t, 10, got["B"])
}

func TestClientWithEndpointsOption(t *testing.T) {
	_ = startTaggedServer(t, ":19103", "X")
	_ = startTaggedServer(t, ":19104", "Y")

	cli := NewClient("tcp", "127.0.0.1:19103",
		WithEndpoints("127.0.0.1:19104"),
		WithHeartbeat(0, 0),
		WithBalancerPolicy(PolicyRoundRobin),
	)
	defer func() { _ = cli.Close() }()

	got := map[string]int{}
	for i := 0; i < 10; i++ {
		resp, err := cli.Call(context.Background(), &Request{Body: []byte("x")})
		require.NoError(t, err)
		got[string(resp.Body)]++
	}
	assert.Greater(t, got["X"], 0)
	assert.Greater(t, got["Y"], 0)
}

func TestClientFailoverUnhealthyEndpoint(t *testing.T) {
	_ = startTaggedServer(t, ":19105", "GOOD")

	// 第一个地址无效，第二个有效
	cli := NewClient("tcp", "127.0.0.1:1,127.0.0.1:19105",
		WithHeartbeat(0, 0),
		WithDialTimeout(100*time.Millisecond),
		WithRequestTimeout(2*time.Second),
		WithRetry(3, 20*time.Millisecond),
		WithEndpointHealth(1, 500*time.Millisecond),
		WithBalancerPolicy(PolicyRoundRobin),
	)
	defer func() { _ = cli.Close() }()

	// 多轮调用应能打到 GOOD（坏节点熔断后只走好节点）
	var success int
	for i := 0; i < 8; i++ {
		resp, err := cli.Call(context.Background(), &Request{Body: []byte("x")})
		if err == nil && string(resp.Body) == "GOOD" {
			success++
		}
	}
	assert.GreaterOrEqual(t, success, 4, "should recover via healthy endpoint")

	// 统计里应能看到两个 endpoint
	st := cli.EndpointStats()
	require.Len(t, st, 2)
}

func TestClientWeightedEndpoints(t *testing.T) {
	_ = startTaggedServer(t, ":19106", "W3")
	_ = startTaggedServer(t, ":19107", "W1")

	cli := NewClient("tcp", "127.0.0.1:19106@3,127.0.0.1:19107@1",
		WithHeartbeat(0, 0),
		WithBalancerPolicy(PolicyWeightedRoundRobin),
		WithMaxOpenConns(4),
	)
	defer func() { _ = cli.Close() }()

	got := map[string]int{}
	const n = 200
	for i := 0; i < n; i++ {
		resp, err := cli.Call(context.Background(), &Request{Body: []byte("x")})
		require.NoError(t, err)
		got[string(resp.Body)]++
	}
	// 约 3:1
	ratio := float64(got["W3"]) / float64(got["W1"]+1)
	assert.InDelta(t, 3.0, ratio, 1.0, "got=%v", got)
}

func TestClientLeastActive(t *testing.T) {
	// 慢节点 A，快节点 B：least_active 应更多打到 B
	slow := HandlerFunc(func(ctx context.Context, req *Request) (*Response, error) {
		time.Sleep(80 * time.Millisecond)
		return &Response{Body: []byte("SLOW")}, nil
	})
	fast := HandlerFunc(func(ctx context.Context, req *Request) (*Response, error) {
		return &Response{Body: []byte("FAST")}, nil
	})

	s1 := NewServer("tcp", ":19108", WithHandler(slow))
	s2 := NewServer("tcp", ":19109", WithHandler(fast))
	go func() { _ = s1.Start() }()
	go func() { _ = s2.Start() }()
	t.Cleanup(func() {
		_ = s1.Shutdown(context.Background())
		_ = s2.Shutdown(context.Background())
	})
	waitListen(t, "127.0.0.1:19108")
	waitListen(t, "127.0.0.1:19109")

	cli := NewClient("tcp", "127.0.0.1:19108,127.0.0.1:19109",
		WithHeartbeat(0, 0),
		WithBalancerPolicy(PolicyLeastActive),
		WithMaxOpenConns(8),
		WithRequestTimeout(3*time.Second),
	)
	defer func() { _ = cli.Close() }()

	var slowN, fastN atomic.Int32
	var wg sync.WaitGroup
	const n = 60
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			resp, err := cli.Call(context.Background(), &Request{Body: []byte("x")})
			if err != nil {
				return
			}
			switch string(resp.Body) {
			case "SLOW":
				slowN.Add(1)
			case "FAST":
				fastN.Add(1)
			}
		}()
		// 错开发起，让 inflight 有机会反映到 least_active 选路
		time.Sleep(3 * time.Millisecond)
	}
	wg.Wait()

	// FAST 应更多（慢节点被 inflight 顶住）
	assert.Greater(t, int(fastN.Load()), int(slowN.Load()),
		"fast=%d slow=%d", fastN.Load(), slowN.Load())
}

func TestClientResolveRefresh(t *testing.T) {
	_ = startTaggedServer(t, ":19110", "OK")

	r := &switchingResolver{
		addrs: []string{"127.0.0.1:19110"},
	}
	cli := NewClient("tcp", "logical:19110",
		WithDNSResolve(true),
		WithResolver(r),
		WithHeartbeat(0, 0),
	)
	defer func() { _ = cli.Close() }()

	require.Equal(t, []string{"127.0.0.1:19110"}, cli.Endpoints())
	resp, err := cli.Call(context.Background(), &Request{Body: []byte("x")})
	require.NoError(t, err)
	assert.Equal(t, "OK", string(resp.Body))

	// 刷新到同一地址仍可用
	require.NoError(t, cli.Resolve(context.Background()))
	resp, err = cli.Call(context.Background(), &Request{Body: []byte("x")})
	require.NoError(t, err)
	assert.Equal(t, "OK", string(resp.Body))
}

type switchingResolver struct {
	addrs []string
}

func (s *switchingResolver) Resolve(_ context.Context, _, address string) ([]string, error) {
	if len(s.addrs) == 0 {
		return nil, fmt.Errorf("no addrs for %s", address)
	}
	return append([]string{}, s.addrs...), nil
}
