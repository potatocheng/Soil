package net

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func startServer(t *testing.T, addr string, opts ...ServerOption) *Server {
	t.Helper()
	serv := NewServer("tcp", addr, opts...)
	go func() { _ = serv.Start() }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = serv.Shutdown(ctx)
	})
	// 仅探测端口就绪，避免慢 handler 拖垮启动
	host := "127.0.0.1" + addr[stringsLastColon(addr):]
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", host, 50*time.Millisecond)
		if err == nil {
			_ = c.Close()
			return serv
		}
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(30 * time.Millisecond)
	return serv
}

func stringsLastColon(addr string) int {
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return i
		}
	}
	return 0
}

func TestClientCommunicate(t *testing.T) {
	_ = startServer(t, ":18188")

	cli := NewClient("tcp", "127.0.0.1:18188", WithHeartbeat(0, 0))
	defer func() { _ = cli.Close() }()

	for i := 0; i < 10; i++ {
		res, err := cli.Communicate("Hello")
		require.NoError(t, err)
		assert.Equal(t, "Hello", res)
	}
}

func TestClientCall(t *testing.T) {
	_ = startServer(t, ":18189")

	cli := NewClient("tcp", "127.0.0.1:18189", WithHeartbeat(0, 0))
	defer func() { _ = cli.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := cli.Call(ctx, &Request{Body: []byte("World")})
	require.NoError(t, err)
	assert.Equal(t, "World", string(resp.Body))
}

func TestClientCallTimeout(t *testing.T) {
	handler := HandlerFunc(func(ctx context.Context, req *Request) (*Response, error) {
		time.Sleep(500 * time.Millisecond)
		return &Response{Body: req.Body}, nil
	})
	_ = startServer(t, ":18190", WithHandler(handler))

	cli := NewClient("tcp", "127.0.0.1:18190",
		WithRequestTimeout(50*time.Millisecond),
		WithHeartbeat(0, 0),
	)
	defer func() { _ = cli.Close() }()

	_, err := cli.Call(context.Background(), &Request{Body: []byte("timeout")})
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestClientOneWay(t *testing.T) {
	var got atomic.Int32
	done := make(chan struct{}, 1)
	handler := HandlerFunc(func(ctx context.Context, req *Request) (*Response, error) {
		got.Add(1)
		select {
		case done <- struct{}{}:
		default:
		}
		return &Response{Body: req.Body}, nil
	})
	_ = startServer(t, ":18194", WithHandler(handler))

	cli := NewClient("tcp", "127.0.0.1:18194", WithRequestTimeout(time.Second), WithHeartbeat(0, 0))
	defer func() { _ = cli.Close() }()

	resp, err := cli.Call(context.Background(), &Request{Body: []byte("ow"), OneWay: true})
	require.NoError(t, err)
	assert.Nil(t, resp.Body)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("handler not called for one-way request")
	}
	assert.Equal(t, int32(1), got.Load())
}

func TestClientMultiplexSingleStream(t *testing.T) {
	// maxOpen=1：所有并发请求必须走同一条 TCP 多路复用连接
	handler := HandlerFunc(func(ctx context.Context, req *Request) (*Response, error) {
		time.Sleep(30 * time.Millisecond)
		return &Response{Body: req.Body}, nil
	})
	_ = startServer(t, ":18195", WithHandler(handler))

	cli := NewClient("tcp", "127.0.0.1:18195",
		WithMaxOpenConns(1),
		WithHeartbeat(0, 0),
		WithRequestTimeout(5*time.Second),
	)
	defer func() { _ = cli.Close() }()

	const n = 40
	var wg sync.WaitGroup
	wg.Add(n)
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
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
	wg.Wait()
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}

	stats := cli.Stats()
	assert.LessOrEqual(t, stats.Open, 1)
	assert.Equal(t, int64(1), stats.DialCount, "should dial only one stream")
}

func TestClientConcurrent(t *testing.T) {
	_ = startServer(t, ":18196")

	cli := NewClient("tcp", "127.0.0.1:18196", WithMaxConns(4), WithHeartbeat(0, 0))
	defer func() { _ = cli.Close() }()

	const n = 50
	var wg sync.WaitGroup
	wg.Add(n)
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			body := []byte{byte(i)}
			resp, err := cli.Call(context.Background(), &Request{Body: body})
			if err != nil {
				errCh <- err
				return
			}
			if string(resp.Body) != string(body) {
				errCh <- errors.New("mismatch")
			}
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}
}

func TestClientHeartbeat(t *testing.T) {
	_ = startServer(t, ":18197")

	cli := NewClient("tcp", "127.0.0.1:18197",
		WithMaxOpenConns(1),
		WithHeartbeat(80*time.Millisecond, 200*time.Millisecond),
		WithIdleTimeout(time.Minute),
	)
	defer func() { _ = cli.Close() }()

	// 先建连
	_, err := cli.Call(context.Background(), &Request{Body: []byte("hi")})
	require.NoError(t, err)

	// 等待至少一轮后台心跳
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cli.Stats().HeartbeatOK > 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("expected heartbeat ok > 0, stats=%+v", cli.Stats())
}

func TestFrameEncodeDecode(t *testing.T) {
	f := NewFrame(MessageTypeRequest, 42, []byte(`{"key":"value"}`), []byte("hello"))
	data, err := f.Encode()
	require.NoError(t, err)

	decoded, err := DecodeFrame(func(buf []byte) error {
		copy(buf, data)
		data = data[len(buf):]
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, f.Magic, decoded.Magic)
	assert.Equal(t, f.Version, decoded.Version)
	assert.Equal(t, f.MsgType, decoded.MsgType)
	assert.Equal(t, f.RequestID, decoded.RequestID)
	assert.Equal(t, f.Header, decoded.Header)
	assert.Equal(t, f.Body, decoded.Body)
}

func TestFrameInvalidMagic(t *testing.T) {
	f := &Frame{Magic: 0x1234, Version: frameVersion, MsgType: MessageTypeRequest, RequestID: 1}
	_, err := f.Encode()
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidMagic)
}

func TestFrameErrorFlag(t *testing.T) {
	f := NewErrorFrame(7, ErrRateLimited)
	assert.True(t, f.IsError())
	data, err := f.Encode()
	require.NoError(t, err)

	decoded, err := DecodeFrame(func(buf []byte) error {
		copy(buf, data)
		data = data[len(buf):]
		return nil
	})
	require.NoError(t, err)
	assert.True(t, decoded.IsError())
	assert.Equal(t, ErrRateLimited.Error(), string(decoded.Body))
}
