package pool

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"
)

type Pool struct {
	// 最大空闲连接数
	MaxIdleCnt int
	// 最大连接容量
	MaxConnCap int
	// 当前连接数
	ConnCnt int
	// 最大空闲时间
	maxIdleTime time.Duration

	// 空闲连接队列
	idleConns chan *idleConn
	// 请求连接阻塞队列
	reqBlockQueue []connRequest

	mutex sync.Mutex
	// 生成连接的工厂函数
	factory func() (net.Conn, error)
	// 检查连接是否有效
	ping func(conn net.Conn) error
}

type idleConn struct {
	conn           net.Conn
	lastActiveTime time.Time
}

type connRequest struct {
	connReqChan chan net.Conn
}

func NewPool(initConnCnt int, maxIdleCnt int, maxConnCap int,
	maxIdleTime time.Duration,
	factory func() (net.Conn, error),
	ping func(net.Conn) error) (*Pool, error) {
	// 检查传入参数是否合法
	if initConnCnt > maxIdleCnt || maxConnCap < maxIdleCnt || initConnCnt < 0 {
		return nil, errors.New("micro: 容量设置错误")
	}
	if factory == nil {
		return nil, errors.New("micro: factory函数不能为nil")
	}
	//if ping == nil {
	//	return nil, errors.New("micro: ping函数不能为nil")
	//}

	idleConns := make(chan *idleConn, initConnCnt)
	for i := 0; i < initConnCnt; i++ {
		conn, err := factory()
		if err != nil {
			return nil, err
		}
		idleConns <- &idleConn{
			conn:           conn,
			lastActiveTime: time.Now(),
		}
	}

	res := &Pool{
		MaxIdleCnt:  maxIdleCnt,
		MaxConnCap:  maxConnCap,
		ConnCnt:     0,
		maxIdleTime: maxIdleTime,
		idleConns:   idleConns,
		factory:     factory,
		ping:        ping,
	}

	return res, nil
}

func (p *Pool) Get(ctx context.Context) (net.Conn, error) {
	for {
		// 尝试从空闲队列中拿连接
		select {
		case idleC := <-p.idleConns:
			// 空闲队列中有连接
			if idleC.lastActiveTime.Add(p.maxIdleTime).Before(time.Now()) {
				// 连接过期, 关闭连接, 尝试拿下一个连接
				_ = idleC.conn.Close()
				continue
			}
			// 判断连接是否失效， 失效则丢弃
			if p.ping != nil {
				err := p.ping(idleC.conn)
				if err != nil {
					_ = idleC.conn.Close()
					continue
				}
			}
			// 没有过期, 连接也没有失效，就返回连接
			return idleC.conn, nil
		default:
			p.mutex.Lock()
			// 空闲队列里没有连接
			if p.ConnCnt >= p.MaxConnCap {
				// 当前连接数超过最大连接容量
				// 阻塞请求
				connReq := connRequest{connReqChan: make(chan net.Conn, 1)}
				p.reqBlockQueue = append(p.reqBlockQueue, connReq)
				p.mutex.Unlock()
				select {
				case <-ctx.Done():
					// 转发请求
					go func() {
						c := <-connReq.connReqChan
						_ = p.Put(c)
					}()
					return nil, ctx.Err()
				case c := <-connReq.connReqChan:
					// 等待归还连接
					return c, nil
				}
			}

			conn, err := p.factory()
			if err != nil {
				return nil, err
			}
			p.ConnCnt++
			p.mutex.Unlock()
			return conn, nil
		}
	}
}

func (p *Pool) Put(conn net.Conn) error {
	p.mutex.Lock()
	if len(p.reqBlockQueue) > 0 {
		// 有阻塞的请求，将连接分配给请求，唤醒请求
		blockReq := p.reqBlockQueue[0]
		p.reqBlockQueue = p.reqBlockQueue[1:]
		p.mutex.Unlock()
		blockReq.connReqChan <- conn
		return nil
	}
	defer p.mutex.Unlock()
	// 没有阻塞请求
	select {
	case p.idleConns <- &idleConn{conn: conn, lastActiveTime: time.Now()}:
		// 空闲连接队列未满
	default:
		_ = conn.Close()
		p.ConnCnt--
	}

	return nil
}

// Release 释放连接池中所有连接
func (p *Pool) Release() {
	p.mutex.Lock()
	conn := p.idleConns
	p.idleConns = nil
	p.mutex.Unlock()

	if conn != nil {
		return
	}

	for c := range conn {
		_ = c.conn.Close()
	}
}
