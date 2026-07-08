package pool

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"
)

func newTestPool(initConnCnt, maxIdleCnt, maxConnCap int) (*Pool, error) {
	return NewPool(initConnCnt, maxIdleCnt, maxConnCap, time.Minute, func() (net.Conn, error) {
		return &mockConn{}, nil
	}, nil)
}

type mockConn struct {
	closed bool
}

func (c *mockConn) Read(b []byte) (n int, err error)   { return 0, nil }
func (c *mockConn) Write(b []byte) (n int, err error)  { return len(b), nil }
func (c *mockConn) Close() error                       { c.closed = true; return nil }
func (c *mockConn) LocalAddr() net.Addr                { return nil }
func (c *mockConn) RemoteAddr() net.Addr               { return nil }
func (c *mockConn) SetDeadline(t time.Time) error      { return nil }
func (c *mockConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *mockConn) SetWriteDeadline(t time.Time) error { return nil }

func TestNewPool(t *testing.T) {
	p, err := newTestPool(1, 10, 20)
	if err != nil {
		t.Fatalf("NewPool failed: %v", err)
	}
	if p.ConnCnt != 1 {
		t.Errorf("expected ConnCnt=1, got %d", p.ConnCnt)
	}
	if len(p.idleConns) != 1 {
		t.Errorf("expected idleConns length=1, got %d", len(p.idleConns))
	}
	if p.closed {
		t.Error("expected closed=false")
	}
}

func TestGetAndPut(t *testing.T) {
	p, err := newTestPool(1, 10, 20)
	if err != nil {
		t.Fatalf("NewPool failed: %v", err)
	}

	conn1, err := p.Get(context.Background())
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if len(p.idleConns) != 0 {
		t.Errorf("expected idleConns length=0 after Get, got %d", len(p.idleConns))
	}

	err = p.Put(conn1)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	if len(p.idleConns) != 1 {
		t.Errorf("expected idleConns length=1 after Put, got %d", len(p.idleConns))
	}
}

func TestGetPoolClosed(t *testing.T) {
	p, err := newTestPool(1, 10, 20)
	if err != nil {
		t.Fatalf("NewPool failed: %v", err)
	}

	p.Release()

	_, err = p.Get(context.Background())
	if err == nil {
		t.Error("expected error when Get from closed pool")
	}
}

func TestPutPoolClosed(t *testing.T) {
	p, err := newTestPool(1, 10, 20)
	if err != nil {
		t.Fatalf("NewPool failed: %v", err)
	}

	conn, err := p.Get(context.Background())
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	p.Release()

	err = p.Put(conn)
	if err == nil {
		t.Error("expected error when Put to closed pool")
	}
}

func TestReleaseIdempotent(t *testing.T) {
	p, err := newTestPool(1, 10, 20)
	if err != nil {
		t.Fatalf("NewPool failed: %v", err)
	}

	p.Release()
	p.Release()

	if !p.closed {
		t.Error("expected closed=true after Release")
	}
}

func TestReleaseClosesAllConnections(t *testing.T) {
	var conn1, conn2 *mockConn
	p, err := NewPool(2, 10, 20, time.Minute, func() (net.Conn, error) {
		c := &mockConn{}
		if conn1 == nil {
			conn1 = c
		} else {
			conn2 = c
		}
		return c, nil
	}, nil)
	if err != nil {
		t.Fatalf("NewPool failed: %v", err)
	}

	_, err = p.Get(context.Background())
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	_, err = p.Get(context.Background())
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	p.Release()

	if !conn1.closed {
		t.Error("expected conn1 closed")
	}
	if !conn2.closed {
		t.Error("expected conn2 closed")
	}
}

func TestReleaseWakesBlockedRequests(t *testing.T) {
	p, err := newTestPool(1, 1, 1)
	if err != nil {
		t.Fatalf("NewPool failed: %v", err)
	}

	_, err = p.Get(context.Background())
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	done := make(chan struct{})
	go func() {
		_, err := p.Get(context.Background())
		if err == nil {
			t.Error("expected error when Get from closed pool")
		}
		close(done)
	}()

	time.Sleep(time.Millisecond * 100)
	p.Release()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Error("blocked request not woken up")
	}
}

func TestGetTimeout(t *testing.T) {
	p, err := newTestPool(1, 1, 1)
	if err != nil {
		t.Fatalf("NewPool failed: %v", err)
	}

	_, err = p.Get(context.Background())
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*50)
	defer cancel()

	_, err = p.Get(ctx)
	if err == nil {
		t.Error("expected timeout error")
	}
}

func TestConnCntDecrementOnExpire(t *testing.T) {
	p, err := NewPool(1, 10, 20, time.Nanosecond, func() (net.Conn, error) {
		return &mockConn{}, nil
	}, nil)
	if err != nil {
		t.Fatalf("NewPool failed: %v", err)
	}

	time.Sleep(time.Millisecond * 10)

	_, err = p.Get(context.Background())
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if p.ConnCnt != 1 {
		t.Errorf("expected ConnCnt=1 after creating new connection, got %d", p.ConnCnt)
	}
}

func TestConnCntDecrementOnPingFail(t *testing.T) {
	p, err := NewPool(1, 10, 20, time.Minute, func() (net.Conn, error) {
		return &mockConn{}, nil
	}, func(conn net.Conn) error {
		return errors.New("ping failed")
	})
	if err != nil {
		t.Fatalf("NewPool failed: %v", err)
	}

	_, err = p.Get(context.Background())
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if p.ConnCnt != 1 {
		t.Errorf("expected ConnCnt=1 after creating new connection, got %d", p.ConnCnt)
	}
}

func TestPutPriorityToBlockedRequest(t *testing.T) {
	p, err := newTestPool(1, 1, 1)
	if err != nil {
		t.Fatalf("NewPool failed: %v", err)
	}

	conn1, err := p.Get(context.Background())
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	done := make(chan *PoolConn, 1)
	go func() {
		conn, _ := p.Get(context.Background())
		done <- conn
	}()

	time.Sleep(time.Millisecond * 100)
	err = p.Put(conn1)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	select {
	case conn := <-done:
		if conn == nil {
			t.Error("expected non-nil connection")
		}
	case <-time.After(time.Second):
		t.Error("blocked request not woken up")
	}
}

func TestConcurrentGetPut(t *testing.T) {
	p, err := newTestPool(10, 10, 100)
	if err != nil {
		t.Fatalf("NewPool failed: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := p.Get(context.Background())
			if err != nil {
				t.Errorf("Get failed: %v", err)
				return
			}
			time.Sleep(time.Millisecond * 5)
			err = p.Put(conn)
			if err != nil {
				t.Errorf("Put failed: %v", err)
			}
		}()
	}

	wg.Wait()

	if p.ConnCnt > 100 {
		t.Errorf("expected ConnCnt <= 100, got %d", p.ConnCnt)
	}
}

func TestIdempotentPoolCapacity(t *testing.T) {
	p, err := newTestPool(5, 10, 20)
	if err != nil {
		t.Fatalf("NewPool failed: %v", err)
	}

	for i := 0; i < 20; i++ {
		conn, err := p.Get(context.Background())
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		err = p.Put(conn)
		if err != nil {
			t.Fatalf("Put failed: %v", err)
		}
	}

	if p.ConnCnt != 5 {
		t.Errorf("expected ConnCnt=5, got %d", p.ConnCnt)
	}
	if len(p.idleConns) != 5 {
		t.Errorf("expected idleConns length=5, got %d", len(p.idleConns))
	}
}

func TestPoolConnCloseReturnsToPool(t *testing.T) {
	p, err := newTestPool(1, 10, 20)
	if err != nil {
		t.Fatalf("NewPool failed: %v", err)
	}

	conn, err := p.Get(context.Background())
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if len(p.idleConns) != 0 {
		t.Errorf("expected idleConns length=0 after Get, got %d", len(p.idleConns))
	}

	err = conn.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	if len(p.idleConns) != 1 {
		t.Errorf("expected idleConns length=1 after Close, got %d", len(p.idleConns))
	}
}

func TestMarkUnhealthy(t *testing.T) {
	p, err := newTestPool(1, 10, 20)
	if err != nil {
		t.Fatalf("NewPool failed: %v", err)
	}

	conn, err := p.Get(context.Background())
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if p.ConnCnt != 1 {
		t.Errorf("expected ConnCnt=1 after Get, got %d", p.ConnCnt)
	}

	conn.MarkUnhealthy()

	if p.ConnCnt != 0 {
		t.Errorf("expected ConnCnt=0 after MarkUnhealthy, got %d", p.ConnCnt)
	}
}

func TestStats(t *testing.T) {
	p, err := newTestPool(10, 10, 20)
	if err != nil {
		t.Fatalf("NewPool failed: %v", err)
	}

	stats := p.Stats()
	if stats.MaxOpenConnections != 20 {
		t.Errorf("expected MaxOpenConnections=20, got %d", stats.MaxOpenConnections)
	}
	if stats.OpenConnections != 10 {
		t.Errorf("expected OpenConnections=10, got %d", stats.OpenConnections)
	}
	if stats.InUse != 0 {
		t.Errorf("expected InUse=0, got %d", stats.InUse)
	}
	if stats.Idle != 10 {
		t.Errorf("expected Idle=10, got %d", stats.Idle)
	}

	conn, err := p.Get(context.Background())
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	stats = p.Stats()
	if stats.InUse != 1 {
		t.Errorf("expected InUse=1 after Get, got %d", stats.InUse)
	}
	if stats.Idle != 9 {
		t.Errorf("expected Idle=9 after Get, got %d", stats.Idle)
	}

	err = p.Put(conn)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	stats = p.Stats()
	if stats.InUse != 0 {
		t.Errorf("expected InUse=0 after Put, got %d", stats.InUse)
	}
	if stats.Idle != 10 {
		t.Errorf("expected Idle=10 after Put, got %d", stats.Idle)
	}
}

func TestMaxConnLifetimePreserved(t *testing.T) {
	p, err := NewPool(1, 10, 20, time.Minute, func() (net.Conn, error) {
		return &mockConn{}, nil
	}, nil, time.Minute)
	if err != nil {
		t.Fatalf("NewPool failed: %v", err)
	}

	conn1, err := p.Get(context.Background())
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	createTime1 := conn1.createTime

	err = conn1.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	conn2, err := p.Get(context.Background())
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if conn2.createTime != createTime1 {
		t.Errorf("expected createTime preserved, got different times")
	}
}

func TestPutWithPoolConnPreservesMetadata(t *testing.T) {
	p, err := NewPool(1, 10, 20, time.Minute, func() (net.Conn, error) {
		return &mockConn{}, nil
	}, nil, time.Minute)
	if err != nil {
		t.Fatalf("NewPool failed: %v", err)
	}

	conn1, err := p.Get(context.Background())
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	createTime1 := conn1.createTime

	time.Sleep(time.Millisecond * 10)

	err = p.Put(conn1)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	conn2, err := p.Get(context.Background())
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if conn2.createTime != createTime1 {
		t.Errorf("expected createTime preserved through Put, got different times")
	}
}

func TestDoubleCloseSafe(t *testing.T) {
	p, err := newTestPool(1, 10, 20)
	if err != nil {
		t.Fatalf("NewPool failed: %v", err)
	}

	conn, err := p.Get(context.Background())
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Fatalf("First Close failed: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Fatalf("Second Close should not fail: %v", err)
	}

	if len(p.idleConns) != 1 {
		t.Errorf("expected idleConns length=1 after double Close, got %d", len(p.idleConns))
	}
	if p.ConnCnt != 1 {
		t.Errorf("expected ConnCnt=1 after double Close, got %d", p.ConnCnt)
	}
}

func TestCloseAndPutSafe(t *testing.T) {
	p, err := newTestPool(1, 10, 20)
	if err != nil {
		t.Fatalf("NewPool failed: %v", err)
	}

	conn, err := p.Get(context.Background())
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	err = p.Put(conn)
	if err != nil {
		t.Fatalf("Put after Close should not fail: %v", err)
	}

	if len(p.idleConns) != 1 {
		t.Errorf("expected idleConns length=1 after Close+Put, got %d", len(p.idleConns))
	}
	if p.ConnCnt != 1 {
		t.Errorf("expected ConnCnt=1 after Close+Put, got %d", p.ConnCnt)
	}
}

func TestMarkUnhealthyAfterCloseSafe(t *testing.T) {
	p, err := newTestPool(1, 10, 20)
	if err != nil {
		t.Fatalf("NewPool failed: %v", err)
	}

	conn, err := p.Get(context.Background())
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	conn.MarkUnhealthy()

	if len(p.idleConns) != 1 {
		t.Errorf("expected idleConns length=1 after Close+MarkUnhealthy, got %d", len(p.idleConns))
	}
	if p.ConnCnt != 1 {
		t.Errorf("expected ConnCnt=1 after Close+MarkUnhealthy, got %d", p.ConnCnt)
	}
}

func BenchmarkGetPut(b *testing.B) {
	p, err := newTestPool(10, 10, 100)
	if err != nil {
		b.Fatalf("NewPool failed: %v", err)
	}
	defer p.Release()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		conn, err := p.Get(context.Background())
		if err != nil {
			b.Fatalf("Get failed: %v", err)
		}
		err = p.Put(conn)
		if err != nil {
			b.Fatalf("Put failed: %v", err)
		}
	}
}

func BenchmarkConcurrentGetPut(b *testing.B) {
	p, err := newTestPool(100, 100, 1000)
	if err != nil {
		b.Fatalf("NewPool failed: %v", err)
	}
	defer p.Release()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			conn, err := p.Get(context.Background())
			if err != nil {
				b.Fatalf("Get failed: %v", err)
			}
			err = p.Put(conn)
			if err != nil {
				b.Fatalf("Put failed: %v", err)
			}
		}
	})
}
