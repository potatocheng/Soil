package pool

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

var (
	errPoolClosed = errors.New("micro: 连接池已关闭")
)

type Pool struct {
	maxIdleCnt      int
	maxConnCap      int
	maxIdleTime     time.Duration
	maxConnLifetime time.Duration

	connCnt     atomic.Int64
	idleConns   []*PoolConn
	activeConns map[*PoolConn]struct{}
	closed      atomic.Bool
	cond        *sync.Cond
	mutex       sync.Mutex
	factory     func() (net.Conn, error)
	ping        func(net.Conn) error
	closeCh     chan struct{}
	wg          sync.WaitGroup
	connPool    sync.Pool

	waitCount         atomic.Int64
	waitDurationNs    atomic.Int64
	maxIdleClosed     atomic.Int64
	maxLifetimeClosed atomic.Int64
}

type Stats struct {
	MaxOpenConnections int
	OpenConnections    int
	InUse              int
	Idle               int
	WaitCount          int64
	WaitDuration       time.Duration
	MaxIdleClosed      int64
	MaxLifetimeClosed  int64
}

type PoolConn struct {
	conn           net.Conn
	createTime     time.Time
	lastActiveTime time.Time
	pool           *Pool
	returned       atomic.Bool
}

func (pc *PoolConn) Read(b []byte) (n int, err error) {
	return pc.conn.Read(b)
}

func (pc *PoolConn) Write(b []byte) (n int, err error) {
	return pc.conn.Write(b)
}

func (pc *PoolConn) Close() error {
	pc.pool.putConn(pc)
	return nil
}

func (pc *PoolConn) LocalAddr() net.Addr {
	return pc.conn.LocalAddr()
}

func (pc *PoolConn) RemoteAddr() net.Addr {
	return pc.conn.RemoteAddr()
}

func (pc *PoolConn) SetDeadline(t time.Time) error {
	return pc.conn.SetDeadline(t)
}

func (pc *PoolConn) SetReadDeadline(t time.Time) error {
	return pc.conn.SetReadDeadline(t)
}

func (pc *PoolConn) SetWriteDeadline(t time.Time) error {
	return pc.conn.SetWriteDeadline(t)
}

func (pc *PoolConn) MarkUnhealthy() {
	pc.pool.markUnhealthy(pc)
}

func NewPool(initConnCnt int, maxIdleCnt int, maxConnCap int,
	maxIdleTime time.Duration,
	factory func() (net.Conn, error),
	ping func(net.Conn) error,
	maxConnLifetime ...time.Duration) (*Pool, error) {
	if initConnCnt > maxIdleCnt || maxConnCap < maxIdleCnt || initConnCnt < 0 {
		return nil, errors.New("micro: 容量设置错误")
	}
	if factory == nil {
		return nil, errors.New("micro: factory函数不能为nil")
	}

	mcl := time.Duration(0)
	if len(maxConnLifetime) > 0 {
		mcl = maxConnLifetime[0]
	}

	p := &Pool{
		maxIdleCnt:      maxIdleCnt,
		maxConnCap:      maxConnCap,
		maxIdleTime:     maxIdleTime,
		maxConnLifetime: mcl,
		factory:         factory,
		ping:            ping,
		closeCh:         make(chan struct{}),
		idleConns:       make([]*PoolConn, 0, maxIdleCnt),
		activeConns:     make(map[*PoolConn]struct{}),
	}
	p.cond = sync.NewCond(&p.mutex)
	p.connPool.New = func() any {
		return &PoolConn{}
	}

	for i := 0; i < initConnCnt; i++ {
		conn, err := factory()
		if err != nil {
			for _, pc := range p.idleConns {
				_ = pc.conn.Close()
			}
			return nil, err
		}
		p.idleConns = append(p.idleConns, p.wrapConn(conn))
	}
	p.connCnt.Store(int64(initConnCnt))

	p.wg.Add(1)
	go p.backgroundCleanup()

	return p, nil
}

func (p *Pool) Stats() Stats {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	return Stats{
		MaxOpenConnections: p.maxConnCap,
		OpenConnections:    int(p.connCnt.Load()),
		InUse:              len(p.activeConns),
		Idle:               len(p.idleConns),
		WaitCount:          p.waitCount.Load(),
		WaitDuration:       time.Duration(p.waitDurationNs.Load()),
		MaxIdleClosed:      p.maxIdleClosed.Load(),
		MaxLifetimeClosed:  p.maxLifetimeClosed.Load(),
	}
}

func (p *Pool) backgroundCleanup() {
	defer p.wg.Done()

	interval := p.maxIdleTime / 2
	if interval < 100*time.Millisecond {
		interval = 100 * time.Millisecond
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			now := time.Now()
			var toClose []*PoolConn
			var idleClosed, lifetimeClosed int64

			p.mutex.Lock()
			if p.closed.Load() {
				p.mutex.Unlock()
				return
			}
			for i := len(p.idleConns) - 1; i >= 0; i-- {
				pc := p.idleConns[i]
				if pc.lastActiveTime.Add(p.maxIdleTime).Before(now) {
					toClose = append(toClose, pc)
					p.idleConns = append(p.idleConns[:i], p.idleConns[i+1:]...)
					idleClosed++
					continue
				}
				if p.maxConnLifetime > 0 && pc.createTime.Add(p.maxConnLifetime).Before(now) {
					toClose = append(toClose, pc)
					p.idleConns = append(p.idleConns[:i], p.idleConns[i+1:]...)
					lifetimeClosed++
				}
			}
			p.mutex.Unlock()

			for _, pc := range toClose {
				p.closeConn(pc)
			}
			if idleClosed > 0 {
				p.maxIdleClosed.Add(idleClosed)
			}
			if lifetimeClosed > 0 {
				p.maxLifetimeClosed.Add(lifetimeClosed)
			}
			if len(toClose) > 0 {
				p.cond.Broadcast()
			}
		case <-p.closeCh:
			return
		}
	}
}

func (p *Pool) Get(ctx context.Context) (*PoolConn, error) {
	p.mutex.Lock()

	if p.closed.Load() {
		p.mutex.Unlock()
		return nil, errPoolClosed
	}

	for {
		pc, err := p.getIdleConnLocked()
		if err != nil {
			p.mutex.Unlock()
			return nil, err
		}
		if pc != nil {
			p.activeConns[pc] = struct{}{}
			p.mutex.Unlock()
			return pc, nil
		}

		if p.connCnt.Load() < int64(p.maxConnCap) {
			p.connCnt.Add(1)
			p.mutex.Unlock()

			conn, err := p.factory()
			if err != nil {
				p.connCnt.Add(-1)
				p.cond.Broadcast()
				return nil, err
			}

			pc := p.wrapConn(conn)
			p.mutex.Lock()
			if p.closed.Load() {
				p.mutex.Unlock()
				p.closeConn(pc)
				return nil, errPoolClosed
			}
			p.activeConns[pc] = struct{}{}
			p.mutex.Unlock()
			return pc, nil
		}

		p.waitCount.Add(1)
		start := time.Now()
		err = p.waitWithContext(ctx)
		p.waitDurationNs.Add(time.Since(start).Nanoseconds())
		if err != nil {
			p.mutex.Unlock()
			return nil, err
		}
	}
}

func (p *Pool) waitWithContext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	stop := context.AfterFunc(ctx, func() {
		p.cond.Broadcast()
	})
	defer stop()

	p.cond.Wait()

	if p.closed.Load() {
		return errPoolClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func (p *Pool) getIdleConnLocked() (*PoolConn, error) {
	now := time.Now()
	for len(p.idleConns) > 0 {
		idx := len(p.idleConns) - 1
		pc := p.idleConns[idx]
		p.idleConns = p.idleConns[:idx]

		if pc.lastActiveTime.Add(p.maxIdleTime).Before(now) {
			p.closeConnLocked(pc)
			p.maxIdleClosed.Add(1)
			continue
		}

		if p.maxConnLifetime > 0 && pc.createTime.Add(p.maxConnLifetime).Before(now) {
			p.closeConnLocked(pc)
			p.maxLifetimeClosed.Add(1)
			continue
		}

		if p.ping != nil {
			p.mutex.Unlock()
			err := p.ping(pc.conn)
			p.mutex.Lock()
			if p.closed.Load() {
				p.closeConnLocked(pc)
				return nil, errPoolClosed
			}
			if err != nil {
				p.closeConnLocked(pc)
				continue
			}
		}

		pc.returned.Store(false)
		return pc, nil
	}
	return nil, nil
}

func (p *Pool) Put(conn net.Conn) error {
	if pc, ok := conn.(*PoolConn); ok {
		return p.putConn(pc)
	}

	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.closed.Load() {
		_ = conn.Close()
		return errPoolClosed
	}

	p.connCnt.Add(1)
	pc := p.wrapConn(conn)

	if len(p.idleConns) < p.maxIdleCnt {
		p.idleConns = append(p.idleConns, pc)
		p.cond.Signal()
		return nil
	}

	p.closeConnLocked(pc)
	return nil
}

func (p *Pool) putConn(pc *PoolConn) error {
	if !pc.returned.CompareAndSwap(false, true) {
		return nil
	}

	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.closed.Load() {
		p.closeConnLocked(pc)
		return errPoolClosed
	}

	delete(p.activeConns, pc)

	if p.maxConnLifetime > 0 && time.Since(pc.createTime) > p.maxConnLifetime {
		p.closeConnLocked(pc)
		p.maxLifetimeClosed.Add(1)
		p.cond.Broadcast()
		return nil
	}

	if p.ping != nil {
		p.mutex.Unlock()
		err := p.ping(pc.conn)
		p.mutex.Lock()
		if err != nil {
			p.closeConnLocked(pc)
			p.cond.Broadcast()
			return nil
		}
	}

	pc.lastActiveTime = time.Now()
	if len(p.idleConns) < p.maxIdleCnt {
		p.idleConns = append(p.idleConns, pc)
		p.cond.Signal()
	} else {
		p.closeConnLocked(pc)
		p.cond.Broadcast()
	}
	return nil
}

func (p *Pool) markUnhealthy(pc *PoolConn) {
	if !pc.returned.CompareAndSwap(false, true) {
		return
	}

	p.mutex.Lock()
	defer p.mutex.Unlock()

	delete(p.activeConns, pc)
	p.closeConnLocked(pc)
	p.cond.Broadcast()
}

func (p *Pool) Release() {
	p.mutex.Lock()
	if p.closed.Load() {
		p.mutex.Unlock()
		return
	}
	p.closed.Store(true)
	p.mutex.Unlock()

	close(p.closeCh)
	p.cond.Broadcast()
	p.wg.Wait()

	p.mutex.Lock()
	for _, pc := range p.idleConns {
		p.closeConnLocked(pc)
	}
	p.idleConns = nil

	for pc := range p.activeConns {
		p.closeConnLocked(pc)
	}
	p.activeConns = nil

	p.connCnt.Store(0)
	p.mutex.Unlock()
}

func (p *Pool) wrapConn(conn net.Conn) *PoolConn {
	pc := p.connPool.Get().(*PoolConn)
	now := time.Now()
	pc.conn = conn
	pc.createTime = now
	pc.lastActiveTime = now
	pc.pool = p
	pc.returned.Store(false)
	return pc
}

func (p *Pool) closeConnLocked(pc *PoolConn) {
	if pc.conn != nil {
		_ = pc.conn.Close()
		pc.conn = nil
	}
	p.connPool.Put(pc)
	p.connCnt.Add(-1)
}

func (p *Pool) closeConn(pc *PoolConn) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.closeConnLocked(pc)
}
