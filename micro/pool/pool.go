package pool

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type Pool struct {
	MaxIdleCnt      int
	MaxConnCap      int
	ConnCnt         int
	maxIdleTime     time.Duration
	maxConnLifetime time.Duration

	idleConns   []*PoolConn
	activeConns map[*PoolConn]struct{}
	closed      bool
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

	res := &Pool{
		MaxIdleCnt:      maxIdleCnt,
		MaxConnCap:      maxConnCap,
		maxIdleTime:     maxIdleTime,
		maxConnLifetime: mcl,
		factory:         factory,
		ping:            ping,
		closeCh:         make(chan struct{}),
		idleConns:       make([]*PoolConn, 0, maxIdleCnt),
		connPool: sync.Pool{
			New: func() interface{} {
				return &PoolConn{}
			},
		},
	}
	res.cond = sync.NewCond(&res.mutex)

	for i := 0; i < initConnCnt; i++ {
		conn, err := factory()
		if err != nil {
			for _, pc := range res.idleConns {
				_ = pc.conn.Close()
			}
			return nil, err
		}
		now := time.Now()
		pc := res.connPool.Get().(*PoolConn)
		pc.conn = conn
		pc.lastActiveTime = now
		pc.createTime = now
		pc.pool = res
		pc.returned.Store(false)
		res.idleConns = append(res.idleConns, pc)
	}

	res.ConnCnt = initConnCnt
	res.activeConns = make(map[*PoolConn]struct{})

	res.wg.Add(1)
	go res.backgroundCleanup()

	return res, nil
}

func (p *Pool) Stats() Stats {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	return Stats{
		MaxOpenConnections: p.MaxConnCap,
		OpenConnections:    p.ConnCnt,
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
			p.mutex.Lock()
			if p.closed {
				p.mutex.Unlock()
				return
			}

			now := time.Now()
			var toClose []*PoolConn
			var idleClosed, lifetimeClosed int64

			idleLen := len(p.idleConns)
			for i := idleLen - 1; i >= 0; i-- {
				pc := p.idleConns[i]

				if pc.lastActiveTime.Add(p.maxIdleTime).Before(now) {
					toClose = append(toClose, pc)
					idleClosed++
					p.idleConns = append(p.idleConns[:i], p.idleConns[i+1:]...)
					continue
				}
				if p.maxConnLifetime > 0 && pc.createTime.Add(p.maxConnLifetime).Before(now) {
					toClose = append(toClose, pc)
					lifetimeClosed++
					p.idleConns = append(p.idleConns[:i], p.idleConns[i+1:]...)
					continue
				}
			}

			p.ConnCnt -= int(idleClosed + lifetimeClosed)
			p.mutex.Unlock()

			for _, pc := range toClose {
				_ = pc.conn.Close()
				pc.conn = nil
				p.connPool.Put(pc)
			}
			p.maxIdleClosed.Add(idleClosed)
			p.maxLifetimeClosed.Add(lifetimeClosed)

			p.cond.Signal()
		case <-p.closeCh:
			return
		}
	}
}

func (p *Pool) Get(ctx context.Context) (*PoolConn, error) {
	p.mutex.Lock()

	if p.closed {
		p.mutex.Unlock()
		return nil, errors.New("micro: 连接池已关闭")
	}

	for {
		for len(p.idleConns) > 0 {
			idleLen := len(p.idleConns)
			pc := p.idleConns[idleLen-1]
			p.idleConns = p.idleConns[:idleLen-1]

			now := time.Now()
			if pc.lastActiveTime.Add(p.maxIdleTime).Before(now) {
				_ = pc.conn.Close()
				pc.conn = nil
				p.connPool.Put(pc)
				p.ConnCnt--
				p.maxIdleClosed.Add(1)
				continue
			}

			if p.maxConnLifetime > 0 && pc.createTime.Add(p.maxConnLifetime).Before(now) {
				_ = pc.conn.Close()
				pc.conn = nil
				p.connPool.Put(pc)
				p.ConnCnt--
				p.maxLifetimeClosed.Add(1)
				continue
			}

			if p.ping != nil {
				p.mutex.Unlock()
				if err := p.ping(pc.conn); err != nil {
					_ = pc.conn.Close()
					pc.conn = nil
					p.connPool.Put(pc)
					p.mutex.Lock()
					p.ConnCnt--
					continue
				}
				p.mutex.Lock()
			}

			pc.returned.Store(false)
			p.activeConns[pc] = struct{}{}
			p.mutex.Unlock()
			return pc, nil
		}

		if p.ConnCnt < p.MaxConnCap {
			p.ConnCnt++
			p.mutex.Unlock()

			conn, err := p.factory()
			if err != nil {
				p.mutex.Lock()
				p.ConnCnt--
				p.mutex.Unlock()
				return nil, err
			}

			now := time.Now()
			pc := p.connPool.Get().(*PoolConn)
			pc.conn = conn
			pc.lastActiveTime = now
			pc.createTime = now
			pc.pool = p
			pc.returned.Store(false)

			p.mutex.Lock()
			p.activeConns[pc] = struct{}{}
			p.mutex.Unlock()
			return pc, nil
		}

		if deadline, ok := ctx.Deadline(); ok {
			timeout := time.Until(deadline)
			if timeout <= 0 {
				p.mutex.Unlock()
				return nil, ctx.Err()
			}
			p.waitCount.Add(1)
			start := time.Now()
			p.mutex.Unlock()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(timeout):
				p.mutex.Lock()
				p.waitDurationNs.Add(time.Since(start).Nanoseconds())
				if p.closed {
					p.mutex.Unlock()
					return nil, errors.New("micro: 连接池已关闭")
				}
				p.mutex.Unlock()
				return nil, errors.New("micro: 获取连接超时")
			}
		}

		p.waitCount.Add(1)
		start := time.Now()
		p.cond.Wait()
		p.waitDurationNs.Add(time.Since(start).Nanoseconds())

		if p.closed {
			p.mutex.Unlock()
			return nil, errors.New("micro: 连接池已关闭")
		}
	}
}

func (p *Pool) Put(conn net.Conn) error {
	if pc, ok := conn.(*PoolConn); ok {
		return p.putConn(pc)
	}

	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.closed {
		_ = conn.Close()
		return errors.New("micro: 连接池已关闭")
	}

	now := time.Now()
	if len(p.idleConns) < p.MaxIdleCnt {
		pc := p.connPool.Get().(*PoolConn)
		pc.conn = conn
		pc.lastActiveTime = now
		pc.createTime = now
		pc.pool = p
		pc.returned.Store(false)
		p.idleConns = append(p.idleConns, pc)
	} else {
		_ = conn.Close()
		p.ConnCnt--
	}

	p.cond.Signal()
	return nil
}

func (p *Pool) putConn(pc *PoolConn) error {
	if !pc.returned.CompareAndSwap(false, true) {
		return nil
	}

	p.mutex.Lock()

	if p.closed {
		p.mutex.Unlock()
		if pc.conn != nil {
			_ = pc.conn.Close()
			pc.conn = nil
		}
		p.connPool.Put(pc)
		return errors.New("micro: 连接池已关闭")
	}

	delete(p.activeConns, pc)

	var toClose *PoolConn
	var closeConn bool

	if p.maxConnLifetime > 0 && time.Now().Sub(pc.createTime) > p.maxConnLifetime {
		toClose = pc
		closeConn = true
		p.ConnCnt--
		p.maxLifetimeClosed.Add(1)
		p.mutex.Unlock()
		p.cond.Signal()
	} else if p.ping != nil {
		p.mutex.Unlock()
		if err := p.ping(pc.conn); err != nil {
			toClose = pc
			closeConn = true
			p.mutex.Lock()
			p.ConnCnt--
			p.mutex.Unlock()
			p.cond.Signal()
		} else {
			pc.lastActiveTime = time.Now()
			p.mutex.Lock()
			if len(p.idleConns) < p.MaxIdleCnt {
				p.idleConns = append(p.idleConns, pc)
			} else {
				toClose = pc
				closeConn = true
				p.ConnCnt--
			}
			p.mutex.Unlock()
			p.cond.Signal()
		}
	} else {
		pc.lastActiveTime = time.Now()
		if len(p.idleConns) < p.MaxIdleCnt {
			p.idleConns = append(p.idleConns, pc)
		} else {
			toClose = pc
			closeConn = true
			p.ConnCnt--
		}
		p.mutex.Unlock()
		p.cond.Signal()
	}

	if closeConn && toClose != nil {
		_ = toClose.conn.Close()
		toClose.conn = nil
		p.connPool.Put(toClose)
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
	_ = pc.conn.Close()
	pc.conn = nil
	p.connPool.Put(pc)
	p.ConnCnt--
	p.cond.Signal()
}

func (p *Pool) Release() {
	p.mutex.Lock()
	if p.closed {
		p.mutex.Unlock()
		return
	}
	p.closed = true
	p.mutex.Unlock()

	close(p.closeCh)
	p.wg.Wait()

	p.mutex.Lock()
	for _, pc := range p.idleConns {
		_ = pc.conn.Close()
		pc.conn = nil
		p.connPool.Put(pc)
	}
	p.idleConns = nil

	for pc := range p.activeConns {
		_ = pc.conn.Close()
		pc.conn = nil
		p.connPool.Put(pc)
	}
	p.activeConns = nil

	p.ConnCnt = 0
	p.mutex.Unlock()

	p.cond.Broadcast()
}
