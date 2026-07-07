package pool

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"
)

type Pool struct {
	MaxIdleCnt      int
	MaxConnCap      int
	ConnCnt         int
	maxIdleTime     time.Duration
	maxConnLifetime time.Duration

	idleConns   []*idleConn
	activeConns map[net.Conn]struct{}
	closed      bool
	cond        *sync.Cond
	mutex       sync.Mutex
	factory     func() (net.Conn, error)
	ping        func(net.Conn) error
	closeCh     chan struct{}
	wg          sync.WaitGroup
}

type idleConn struct {
	conn           net.Conn
	lastActiveTime time.Time
	createTime     time.Time
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

	idleConns := make([]*idleConn, 0, maxIdleCnt)
	for i := 0; i < initConnCnt; i++ {
		conn, err := factory()
		if err != nil {
			for _, c := range idleConns {
				_ = c.conn.Close()
			}
			return nil, err
		}
		now := time.Now()
		idleConns = append(idleConns, &idleConn{
			conn:           conn,
			lastActiveTime: now,
			createTime:     now,
		})
	}

	res := &Pool{
		MaxIdleCnt:      maxIdleCnt,
		MaxConnCap:      maxConnCap,
		ConnCnt:         initConnCnt,
		maxIdleTime:     maxIdleTime,
		maxConnLifetime: mcl,
		idleConns:       idleConns,
		activeConns:     make(map[net.Conn]struct{}),
		factory:         factory,
		ping:            ping,
		closeCh:         make(chan struct{}),
	}
	res.cond = sync.NewCond(&res.mutex)

	res.wg.Add(1)
	go res.backgroundCleanup()

	return res, nil
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
			newIdleConns := make([]*idleConn, 0, len(p.idleConns))
			for _, c := range p.idleConns {
				if c.lastActiveTime.Add(p.maxIdleTime).Before(now) {
					_ = c.conn.Close()
					p.ConnCnt--
					continue
				}
				if p.maxConnLifetime > 0 && c.createTime.Add(p.maxConnLifetime).Before(now) {
					_ = c.conn.Close()
					p.ConnCnt--
					continue
				}
				newIdleConns = append(newIdleConns, c)
			}
			p.idleConns = newIdleConns

			p.mutex.Unlock()
			p.cond.Signal()
		case <-p.closeCh:
			return
		}
	}
}

func (p *Pool) Get(ctx context.Context) (net.Conn, error) {
	p.mutex.Lock()

	if p.closed {
		p.mutex.Unlock()
		return nil, errors.New("micro: 连接池已关闭")
	}

	for {
		for len(p.idleConns) > 0 {
			idleC := p.idleConns[len(p.idleConns)-1]
			p.idleConns = p.idleConns[:len(p.idleConns)-1]

			if idleC.lastActiveTime.Add(p.maxIdleTime).Before(time.Now()) {
				_ = idleC.conn.Close()
				p.ConnCnt--
				continue
			}

			if p.maxConnLifetime > 0 && idleC.createTime.Add(p.maxConnLifetime).Before(time.Now()) {
				_ = idleC.conn.Close()
				p.ConnCnt--
				continue
			}

			if p.ping != nil {
				if err := p.ping(idleC.conn); err != nil {
					_ = idleC.conn.Close()
					p.ConnCnt--
					continue
				}
			}

			p.activeConns[idleC.conn] = struct{}{}
			p.mutex.Unlock()
			return idleC.conn, nil
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

			p.mutex.Lock()
			p.activeConns[conn] = struct{}{}
			p.mutex.Unlock()
			return conn, nil
		}

		if deadline, ok := ctx.Deadline(); ok {
			timeout := time.Until(deadline)
			if timeout <= 0 {
				p.mutex.Unlock()
				return nil, ctx.Err()
			}
			p.mutex.Unlock()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(timeout):
				p.mutex.Lock()
				if p.closed {
					p.mutex.Unlock()
					return nil, errors.New("micro: 连接池已关闭")
				}
				p.mutex.Unlock()
				return nil, errors.New("micro: 获取连接超时")
			}
		}

		p.cond.Wait()

		if p.closed {
			p.mutex.Unlock()
			return nil, errors.New("micro: 连接池已关闭")
		}
	}
}

func (p *Pool) Put(conn net.Conn) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	if p.closed {
		_ = conn.Close()
		return errors.New("micro: 连接池已关闭")
	}

	delete(p.activeConns, conn)

	if len(p.idleConns) < p.MaxIdleCnt {
		p.idleConns = append(p.idleConns, &idleConn{
			conn:           conn,
			lastActiveTime: time.Now(),
			createTime:     time.Now(),
		})
	} else {
		_ = conn.Close()
		p.ConnCnt--
	}

	p.cond.Signal()
	return nil
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
	for _, c := range p.idleConns {
		_ = c.conn.Close()
	}
	p.idleConns = nil

	for conn := range p.activeConns {
		_ = conn.Close()
	}
	p.activeConns = nil

	p.ConnCnt = 0
	p.mutex.Unlock()

	p.cond.Broadcast()
}
