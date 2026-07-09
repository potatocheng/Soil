package net

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerGracefulShutdown(t *testing.T) {
	serv := NewServer("tcp", ":18291")
	go func() { _ = serv.Start() }()
	time.Sleep(50 * time.Millisecond)

	cli := NewClient("tcp", "127.0.0.1:18291", WithHeartbeat(0, 0))
	resp, err := cli.Communicate("before shutdown")
	require.NoError(t, err)
	assert.Equal(t, "before shutdown", resp)
	require.NoError(t, cli.Close())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, serv.Shutdown(ctx))
}

func TestServerDrainInFlight(t *testing.T) {
	// 关闭时在途请求仍应完整返回，而不是被掐断
	started := make(chan struct{})
	handler := HandlerFunc(func(ctx context.Context, req *Request) (*Response, error) {
		close(started)
		time.Sleep(200 * time.Millisecond)
		return &Response{Body: []byte("done")}, nil
	})

	serv := NewServer("tcp", ":18298", WithHandler(handler))
	go func() { _ = serv.Start() }()
	time.Sleep(50 * time.Millisecond)

	cli := NewClient("tcp", "127.0.0.1:18298",
		WithHeartbeat(0, 0),
		WithRequestTimeout(2*time.Second),
	)
	defer func() { _ = cli.Close() }()

	errCh := make(chan error, 1)
	go func() {
		resp, err := cli.Call(context.Background(), &Request{Body: []byte("slow")})
		if err != nil {
			errCh <- err
			return
		}
		if string(resp.Body) != "done" {
			errCh <- errors.New("unexpected body: " + string(resp.Body))
			return
		}
		errCh <- nil
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("handler not started")
	}

	// 在 handler 睡眠期间开始优雅关闭（给足 drain 时间）
	shCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	shDone := make(chan error, 1)
	go func() { shDone <- serv.Shutdown(shCtx) }()

	select {
	case err := <-errCh:
		require.NoError(t, err, "in-flight request should complete during drain")
	case <-time.After(3 * time.Second):
		t.Fatal("call did not finish")
	}

	select {
	case err := <-shDone:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown did not finish")
	}
}

func TestServerHandlerPanic(t *testing.T) {
	handler := HandlerFunc(func(ctx context.Context, req *Request) (*Response, error) {
		panic("intentional panic")
	})

	serv := NewServer("tcp", ":18292", WithHandler(handler))
	go func() { _ = serv.Start() }()
	defer func() { _ = serv.Shutdown(context.Background()) }()
	time.Sleep(50 * time.Millisecond)

	cli := NewClient("tcp", "127.0.0.1:18292", WithHeartbeat(0, 0))
	defer func() { _ = cli.Close() }()

	_, err := cli.Call(context.Background(), &Request{Body: []byte("panic")})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrHandlerPanic)
	assert.ErrorIs(t, err, ErrRemote)
}

func TestServerRateLimiter(t *testing.T) {
	limiter := &fakeLimiter{}
	limiter.allowed.Store(true)

	serv := NewServer("tcp", ":18293", WithRateLimiter(limiter))
	go func() { _ = serv.Start() }()
	defer func() { _ = serv.Shutdown(context.Background()) }()
	time.Sleep(50 * time.Millisecond)

	cli := NewClient("tcp", "127.0.0.1:18293", WithHeartbeat(0, 0))
	defer func() { _ = cli.Close() }()

	limiter.allowed.Store(false)
	_, err := cli.Call(context.Background(), &Request{Body: []byte("limited")})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrRateLimited)
}

func TestServerMiddleware(t *testing.T) {
	var order []string
	handler := HandlerFunc(func(ctx context.Context, req *Request) (*Response, error) {
		order = append(order, "handler")
		return &Response{Body: req.Body}, nil
	})
	mw1 := func(next Handler) Handler {
		return HandlerFunc(func(ctx context.Context, req *Request) (*Response, error) {
			order = append(order, "mw1-in")
			resp, err := next.Handle(ctx, req)
			order = append(order, "mw1-out")
			return resp, err
		})
	}
	mw2 := func(next Handler) Handler {
		return HandlerFunc(func(ctx context.Context, req *Request) (*Response, error) {
			order = append(order, "mw2-in")
			resp, err := next.Handle(ctx, req)
			order = append(order, "mw2-out")
			return resp, err
		})
	}

	serv := NewServer("tcp", ":18296", WithHandler(handler), WithMiddleware(mw1, mw2))
	go func() { _ = serv.Start() }()
	defer func() { _ = serv.Shutdown(context.Background()) }()
	time.Sleep(50 * time.Millisecond)

	cli := NewClient("tcp", "127.0.0.1:18296", WithHeartbeat(0, 0))
	defer func() { _ = cli.Close() }()

	_, err := cli.Call(context.Background(), &Request{Body: []byte("x")})
	require.NoError(t, err)
	assert.Equal(t, []string{"mw1-in", "mw2-in", "handler", "mw2-out", "mw1-out"}, order)
}

func TestServerConcurrentRequests(t *testing.T) {
	handler := HandlerFunc(func(ctx context.Context, req *Request) (*Response, error) {
		time.Sleep(20 * time.Millisecond)
		return &Response{Body: req.Body}, nil
	})
	serv := NewServer("tcp", ":18297", WithHandler(handler))
	go func() { _ = serv.Start() }()
	defer func() { _ = serv.Shutdown(context.Background()) }()
	time.Sleep(50 * time.Millisecond)

	cli := NewClient("tcp", "127.0.0.1:18297", WithMaxConns(4), WithHeartbeat(0, 0))
	defer func() { _ = cli.Close() }()

	const n = 30
	errCh := make(chan error, n)
	var left atomic.Int32
	left.Store(int32(n))
	for i := 0; i < n; i++ {
		go func(i int) {
			defer left.Add(-1)
			body := []byte{byte(i)}
			resp, err := cli.Call(context.Background(), &Request{Body: body})
			if err != nil {
				errCh <- err
				return
			}
			if string(resp.Body) != string(body) {
				errCh <- errors.New("body mismatch")
			}
		}(i)
	}
	for left.Load() > 0 {
		time.Sleep(5 * time.Millisecond)
	}
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}
}

type fakeLimiter struct {
	allowed atomic.Bool
}

func (f *fakeLimiter) Allow(_ context.Context) bool {
	return f.allowed.Load()
}

func TestStreamPoolStatsAndLimit(t *testing.T) {
	serv := NewServer("tcp", ":18299")
	go func() { _ = serv.Start() }()
	defer func() { _ = serv.Shutdown(context.Background()) }()
	time.Sleep(50 * time.Millisecond)

	cli := NewClient("tcp", "127.0.0.1:18299",
		WithMaxOpenConns(2),
		WithHeartbeat(0, 0),
	)
	defer func() { _ = cli.Close() }()

	// 并发请求会创建最多 2 条 stream
	const n = 20
	errCh := make(chan error, n)
	var left atomic.Int32
	left.Store(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer left.Add(-1)
			_, err := cli.Call(context.Background(), &Request{Body: []byte{byte(i)}})
			if err != nil {
				errCh <- err
			}
		}(i)
	}
	for left.Load() > 0 {
		time.Sleep(5 * time.Millisecond)
	}
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}

	st := cli.Stats()
	assert.LessOrEqual(t, st.Open, 2)
	assert.LessOrEqual(t, st.DialCount, int64(2))
	assert.GreaterOrEqual(t, st.DialCount, int64(1))
}

func TestStreamPoolIdleRecycle(t *testing.T) {
	serv := NewServer("tcp", ":18300")
	go func() { _ = serv.Start() }()
	defer func() { _ = serv.Shutdown(context.Background()) }()
	time.Sleep(50 * time.Millisecond)

	cli := NewClient("tcp", "127.0.0.1:18300",
		WithMaxOpenConns(1),
		WithIdleTimeout(100*time.Millisecond),
		WithHeartbeat(0, 0),
	)
	defer func() { _ = cli.Close() }()

	_, err := cli.Call(context.Background(), &Request{Body: []byte("a")})
	require.NoError(t, err)
	require.Equal(t, 1, cli.Stats().Open)

	// 等待后台 idle 回收
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cli.Stats().Open == 0 && cli.Stats().ClosedIdle >= 1 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("stream not recycled: %+v", cli.Stats())
}
