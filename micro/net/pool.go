package net

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// streamPool 管理多路复用 stream：
// - maxOpen：最大并发 stream 数
// - idleTimeout / maxLifetime：后台回收
// - heartbeat：空闲探活
// 同一 stream 可被多个 Call 并发使用。
// 每个 pool 绑定一个固定 dial 地址（多 endpoint 时每节点一个 pool）。
type streamPool struct {
	client  *Client
	address string

	maxOpen     int
	idleTimeout time.Duration
	maxLifetime time.Duration

	heartbeatInterval time.Duration
	heartbeatTimeout  time.Duration

	mu      sync.Mutex
	cond    *sync.Cond
	streams []*stream
	dialing int // 在途 dial，防止惊群超开
	closed  bool

	stopCh chan struct{}
	wg     sync.WaitGroup

	dialCount    atomic.Int64
	closedIdle   atomic.Int64
	closedLife   atomic.Int64
	closedBad    atomic.Int64
	heartbeatOK  atomic.Int64
	heartbeatErr atomic.Int64
}

// PoolStats 连接池快照
type PoolStats struct {
	Open          int
	DialCount     int64
	ClosedIdle    int64
	ClosedLife    int64
	ClosedBad     int64
	HeartbeatOK   int64
	HeartbeatErr  int64
	TotalInflight int32
}

func newStreamPool(c *Client, address string) *streamPool {
	o := c.opts
	maxOpen := o.maxOpenConns
	if maxOpen <= 0 {
		maxOpen = 1
	}
	p := &streamPool{
		client:            c,
		address:           address,
		maxOpen:           maxOpen,
		idleTimeout:       o.idleTimeout,
		maxLifetime:       o.maxConnLifetime,
		heartbeatInterval: o.heartbeatInterval,
		heartbeatTimeout:  o.heartbeatTimeout,
		streams:           make([]*stream, 0, maxOpen),
		stopCh:            make(chan struct{}),
	}
	p.cond = sync.NewCond(&p.mu)
	p.wg.Add(1)
	go p.background()
	return p
}

// Get 获取一条可用 stream（负载最低）；不足则新建；达上限则复用或等待在途 dial。
func (p *streamPool) Get(ctx context.Context) (*stream, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for {
		if p.closed {
			return nil, errPoolClosed
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		p.compactLocked()

		if s := p.pickLocked(); s != nil {
			// 已有 stream：直接复用（多路复用核心）
			// 未满 maxOpen 且全部高负载时仍可再 dial
			if len(p.streams)+p.dialing >= p.maxOpen || s.Inflight() < 32 {
				return s, nil
			}
		}

		if len(p.streams)+p.dialing < p.maxOpen {
			p.dialing++
			p.mu.Unlock()

			s, err := p.dialStream()

			p.mu.Lock()
			p.dialing--
			p.cond.Broadcast()

			if err != nil {
				return nil, err
			}
			if p.closed {
				_ = s.Close()
				return nil, errPoolClosed
			}
			p.compactLocked()
			if len(p.streams) >= p.maxOpen {
				// 极端并发下已满，关掉新建并复用
				_ = s.Close()
				p.client.opts.metrics.ConnectionClosed("client")
				if existing := p.pickLocked(); existing != nil {
					return existing, nil
				}
				continue
			}
			p.streams = append(p.streams, s)
			return s, nil
		}

		// 达上限且当前无可用：等待 stream 出现或 dial 完成
		if s := p.pickLocked(); s != nil {
			return s, nil
		}

		if err := p.waitLocked(ctx); err != nil {
			return nil, err
		}
	}
}

func (p *streamPool) waitLocked(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	stop := context.AfterFunc(ctx, func() {
		p.cond.Broadcast()
	})
	defer stop()
	p.cond.Wait()
	if p.closed {
		return errPoolClosed
	}
	return ctx.Err()
}

func (p *streamPool) dialStream() (*stream, error) {
	conn, err := p.client.dialAddr(p.address)
	if err != nil {
		return nil, err
	}
	p.dialCount.Add(1)
	return newStream(conn, p.client), nil
}

func (p *streamPool) pickLocked() *stream {
	var best *stream
	var bestLoad int32 = -1
	for _, s := range p.streams {
		if !s.Alive() {
			continue
		}
		load := s.Inflight()
		if best == nil || load < bestLoad {
			best = s
			bestLoad = load
		}
	}
	return best
}

func (p *streamPool) compactLocked() {
	n := 0
	for _, s := range p.streams {
		if s.Alive() {
			p.streams[n] = s
			n++
		} else {
			p.closedBad.Add(1)
		}
	}
	p.streams = p.streams[:n]
}

func (p *streamPool) background() {
	defer p.wg.Done()

	interval := 5 * time.Second
	if p.heartbeatInterval > 0 && p.heartbeatInterval < interval {
		interval = p.heartbeatInterval
	}
	if p.idleTimeout > 0 && p.idleTimeout/2 < interval {
		interval = p.idleTimeout / 2
		if interval < 200*time.Millisecond {
			interval = 200 * time.Millisecond
		}
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
			p.maintain()
		}
	}
}

func (p *streamPool) maintain() {
	now := time.Now()
	var toClose []*stream
	var toPing []*stream

	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.compactLocked()

	alive := make([]*stream, 0, len(p.streams))
	for _, s := range p.streams {
		if !s.Alive() {
			p.closedBad.Add(1)
			continue
		}
		if p.maxLifetime > 0 && now.Sub(s.CreatedAt()) > p.maxLifetime && s.Inflight() == 0 {
			toClose = append(toClose, s)
			p.closedLife.Add(1)
			continue
		}
		if p.idleTimeout > 0 && s.Inflight() == 0 && now.Sub(s.LastActive()) > p.idleTimeout {
			toClose = append(toClose, s)
			p.closedIdle.Add(1)
			continue
		}
		if p.heartbeatInterval > 0 && now.Sub(s.LastActive()) >= p.heartbeatInterval {
			toPing = append(toPing, s)
		}
		alive = append(alive, s)
	}
	p.streams = alive
	p.mu.Unlock()

	for _, s := range toClose {
		_ = s.Close()
		p.client.opts.metrics.ConnectionClosed("client")
	}
	p.cond.Broadcast()

	for _, s := range toPing {
		p.pingOne(s)
	}
}

func (p *streamPool) pingOne(s *stream) {
	if !s.Alive() {
		return
	}
	timeout := p.heartbeatTimeout
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := s.Ping(ctx); err != nil {
		p.heartbeatErr.Add(1)
		_ = s.Close()
		p.client.opts.metrics.ConnectionClosed("client")
		p.mu.Lock()
		p.removeLocked(s)
		p.mu.Unlock()
		p.cond.Broadcast()
		return
	}
	p.heartbeatOK.Add(1)
}

func (p *streamPool) removeLocked(target *stream) {
	n := 0
	for _, s := range p.streams {
		if s != target {
			p.streams[n] = s
			n++
		}
	}
	p.streams = p.streams[:n]
}

// Close 关闭池与全部 stream
func (p *streamPool) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	streams := p.streams
	p.streams = nil
	close(p.stopCh)
	p.cond.Broadcast()
	p.mu.Unlock()

	for _, s := range streams {
		_ = s.Close()
		p.client.opts.metrics.ConnectionClosed("client")
	}
	p.wg.Wait()
	return nil
}

// Stats 返回池状态
func (p *streamPool) Stats() PoolStats {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.compactLocked()
	var inflight int32
	for _, s := range p.streams {
		inflight += s.Inflight()
	}
	return PoolStats{
		Open:          len(p.streams),
		DialCount:     p.dialCount.Load(),
		ClosedIdle:    p.closedIdle.Load(),
		ClosedLife:    p.closedLife.Load(),
		ClosedBad:     p.closedBad.Load(),
		HeartbeatOK:   p.heartbeatOK.Load(),
		HeartbeatErr:  p.heartbeatErr.Load(),
		TotalInflight: inflight,
	}
}

// Len 当前打开的 stream 数
func (p *streamPool) Len() int {
	return p.Stats().Open
}

// OpenCount 当前 stream 数
func (p *streamPool) OpenCount() int {
	return p.Stats().Open
}
