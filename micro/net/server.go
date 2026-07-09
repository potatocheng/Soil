package net

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// Server 是生产级 TCP 服务端
type Server struct {
	network  string
	address  string
	opts     *serverOptions
	handler  Handler
	listener net.Listener
	closed   atomic.Bool // 停止 Accept + 进入 drain
	wg       sync.WaitGroup
	conns    sync.Map // net.Conn -> *connSlot
	sem      chan struct{}

	baseCtx context.Context
	cancel  context.CancelFunc
}

// connSlot 管理单连接：写串行化 + 在途请求等待（优雅 drain）
type connSlot struct {
	conn    net.Conn
	writeMu sync.Mutex
	reqWg   sync.WaitGroup // 本连接上未完成的 handleRequest
	// draining 后不再接新请求，但仍可写完在途响应
	draining atomic.Bool
}

// NewServer 创建服务端
func NewServer(network, address string, opts ...ServerOption) *Server {
	o := defaultServerOptions()
	for _, opt := range opts {
		opt(o)
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{
		network: network,
		address: address,
		opts:    o,
		handler: o.buildHandler(),
		sem:     make(chan struct{}, o.maxConns),
		baseCtx: ctx,
		cancel:  cancel,
	}
	return s
}

// Start 启动服务监听（阻塞直到 Shutdown 或致命错误）
func (s *Server) Start() error {
	if s.closed.Load() {
		return errors.New("server: already closed")
	}

	var err error
	if s.opts.tlsConfig != nil {
		s.listener, err = tls.Listen(s.network, s.address, s.opts.tlsConfig)
	} else {
		s.listener, err = net.Listen(s.network, s.address)
	}
	if err != nil {
		return err
	}

	s.opts.logger.Info("server started", "addr", s.listener.Addr().String())

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if s.closed.Load() {
				return nil
			}
			s.opts.logger.Error("accept failed", "error", err)
			continue
		}

		select {
		case s.sem <- struct{}{}:
			s.wg.Add(1)
			go s.handleConn(conn)
		default:
			s.opts.logger.Warn("too many connections", "remote", conn.RemoteAddr())
			_ = conn.Close()
		}
	}
}

// Shutdown 优雅关闭：
//  1. 停止 Accept
//  2. 标记所有连接 draining：停止读新请求，CloseRead 半关闭
//  3. 等待在途 handleRequest 写完响应（此阶段不 cancel 业务 ctx）
//  4. 正常结束后再 cancel；超时则 cancel + 强关剩余连接
func (s *Server) Shutdown(ctx context.Context) error {
	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}
	if s.listener != nil {
		_ = s.listener.Close()
	}

	// 进入 drain：各连接停止接新请求（先不 cancel，避免掐断在途业务）
	s.conns.Range(func(_, value any) bool {
		slot := value.(*connSlot)
		s.beginDrain(slot)
		return true
	})

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		if s.cancel != nil {
			s.cancel()
		}
		return nil
	case <-ctx.Done():
		// 超时：取消在途 ctx 并强关连接
		if s.cancel != nil {
			s.cancel()
		}
		s.conns.Range(func(_, value any) bool {
			slot := value.(*connSlot)
			_ = slot.conn.Close()
			return true
		})
		select {
		case <-done:
		case <-time.After(200 * time.Millisecond):
		}
		return ctx.Err()
	}
}

func (s *Server) beginDrain(slot *connSlot) {
	if !slot.draining.CompareAndSwap(false, true) {
		return
	}
	// 立即打断阻塞中的 Read（跨平台可靠）
	_ = slot.conn.SetReadDeadline(time.Now())
	// 半关闭读侧：阻止新请求进入，仍允许写完响应
	if tc, ok := slot.conn.(*net.TCPConn); ok {
		_ = tc.CloseRead()
	}
}

func (s *Server) handleConn(conn net.Conn) {
	slot := &connSlot{conn: conn}
	s.conns.Store(conn, slot)
	s.opts.metrics.ConnectionOpened("server")

	defer func() {
		// 关键：先等本连接在途请求写完，再关连接（避免 drain 时掐断响应）
		slot.reqWg.Wait()
		s.wg.Done()
		<-s.sem
		_ = conn.Close()
		s.conns.Delete(conn)
		s.opts.metrics.ConnectionClosed("server")
	}()

	// 若启动时已在关闭，直接 drain
	if s.closed.Load() {
		s.beginDrain(slot)
		return
	}

	connCtx, cancel := context.WithCancel(s.baseCtx)
	defer cancel()

	reader := bufio.NewReaderSize(conn, 32*1024)
	lim := s.opts.limits()

	for {
		if s.closed.Load() || slot.draining.Load() {
			s.beginDrain(slot)
			return
		}

		if err := s.setReadDeadline(conn); err != nil {
			return
		}

		frame, err := readFrame(reader, lim)
		if err != nil {
			if s.closed.Load() || slot.draining.Load() || isClosedConnErr(err) {
				return
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				return
			}
			s.opts.logger.Warn("read frame failed", "remote", conn.RemoteAddr(), "error", err)
			return
		}

		if frame.IsHeartbeat() {
			if frame.MsgType == MessageTypePing || frame.MsgType == MessageTypeHeartbeat {
				if err := s.sendFrame(slot, NewFrame(MessageTypePong, frame.RequestID, nil, nil)); err != nil {
					return
				}
			}
			continue
		}

		if frame.MsgType != MessageTypeRequest {
			s.opts.logger.Warn("unexpected message type", "type", frame.MsgType, "remote", conn.RemoteAddr())
			continue
		}

		// drain 期间拒绝新业务请求（防御性；正常应已退出循环）
		if slot.draining.Load() || s.closed.Load() {
			_ = s.sendFrame(slot, NewErrorFrame(frame.RequestID, errors.New("server draining")))
			return
		}

		reqCtx := connCtx
		if !s.opts.limiter.Allow(reqCtx) {
			_ = s.sendFrame(slot, NewErrorFrame(frame.RequestID, ErrRateLimited))
			continue
		}

		req := &Request{
			Ctx:       reqCtx,
			RequestID: frame.RequestID,
			Header:    frame.Header,
			Body:      frame.Body,
			OneWay:    frame.IsOneWay(),
		}

		slot.reqWg.Add(1)
		s.wg.Add(1)
		go s.handleRequest(slot, frame, req)
	}
}

func (s *Server) handleRequest(slot *connSlot, reqFrame *Frame, req *Request) {
	defer s.wg.Done()
	defer slot.reqWg.Done()

	ctx := req.Ctx
	if s.opts.handlerTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.opts.handlerTimeout)
		defer cancel()
	}
	req.Ctx = ctx

	start := time.Now()
	resp, err := s.safeHandle(req)
	s.opts.metrics.RequestHandled(time.Since(start), err)

	if reqFrame.IsOneWay() {
		return
	}

	var out *Frame
	if err != nil {
		out = NewErrorFrame(reqFrame.RequestID, err)
	} else if resp != nil && resp.Error != nil {
		out = NewErrorFrame(reqFrame.RequestID, resp.Error)
	} else {
		out = NewFrame(MessageTypeResponse, reqFrame.RequestID, nil, nil)
		if resp != nil {
			out.Header = resp.Header
			out.Body = resp.Body
		}
	}

	if werr := s.sendFrame(slot, out); werr != nil {
		s.opts.logger.Warn("write response failed", "remote", slot.conn.RemoteAddr(), "error", werr)
	}
}

func (s *Server) safeHandle(req *Request) (resp *Response, err error) {
	defer func() {
		if r := recover(); r != nil {
			s.opts.logger.Error("handler panic", "requestID", req.RequestID, "panic", r)
			err = ErrHandlerPanic
			resp = nil
		}
	}()
	return s.handler.Handle(req.Ctx, req)
}

// sendFrame 在写锁保护下串行写出，避免同连接帧交错（支持 handler 并发 + 客户端多路复用）
func (s *Server) sendFrame(slot *connSlot, f *Frame) error {
	slot.writeMu.Lock()
	defer slot.writeMu.Unlock()

	if s.opts.writeTimeout > 0 {
		_ = slot.conn.SetWriteDeadline(time.Now().Add(s.opts.writeTimeout))
	}
	return writeFrame(slot.conn, f, s.opts.limits())
}

func (s *Server) setReadDeadline(conn net.Conn) error {
	d := s.opts.idleTimeout
	if d <= 0 {
		d = s.opts.readTimeout
	}
	if d <= 0 {
		return conn.SetReadDeadline(time.Time{})
	}
	return conn.SetReadDeadline(time.Now().Add(d))
}
