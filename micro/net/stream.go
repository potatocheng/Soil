package net

import (
	"bufio"
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// rpcResult 是多路复用等待队列中的一次响应
type rpcResult struct {
	frame *Frame
	err   error
}

// stream 是一条支持请求多路复用的长连接：
// - 写路径：writeMu 串行
// - 读路径：单 goroutine readLoop 按 RequestID 分发
// - 心跳：可选 Ping/Pong 探活
type stream struct {
	conn   net.Conn
	reader *bufio.Reader
	client *Client
	lim    frameLimits

	writeMu sync.Mutex

	mu      sync.Mutex
	pending map[uint64]chan rpcResult
	closed  bool
	closeErr error

	inflight   atomic.Int32
	lastActive atomic.Int64 // unix nano
	createdAt  time.Time

	// 通知 readLoop 退出后的清理完成（可选）
	done chan struct{}
}

func newStream(conn net.Conn, c *Client) *stream {
	s := &stream{
		conn:      conn,
		reader:    bufio.NewReaderSize(conn, 32*1024),
		client:    c,
		lim:       c.opts.limits(),
		pending:   make(map[uint64]chan rpcResult),
		createdAt: time.Now(),
		done:      make(chan struct{}),
	}
	s.touch()
	go s.readLoop()
	return s
}

func (s *stream) touch() {
	s.lastActive.Store(time.Now().UnixNano())
}

func (s *stream) LastActive() time.Time {
	return time.Unix(0, s.lastActive.Load())
}

func (s *stream) CreatedAt() time.Time { return s.createdAt }

func (s *stream) Inflight() int32 { return s.inflight.Load() }

func (s *stream) Alive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.closed
}

func (s *stream) readLoop() {
	defer close(s.done)
	for {
		// 心跳/空闲场景下读超时由心跳或 pool 清理负责；这里用较长的绝对 deadline 防永久阻塞
		if d := s.client.opts.readTimeout; d > 0 {
			// 多路复用下单次读可能长时间无数据；用 max(readTimeout, heartbeat*2, idle/2)
			idle := s.client.opts.idleTimeout
			hb := s.client.opts.heartbeatInterval
			wait := d
			if idle > wait {
				wait = idle
			}
			if hb > 0 && hb*3 > wait {
				wait = hb * 3
			}
			_ = s.conn.SetReadDeadline(time.Now().Add(wait))
		} else {
			_ = s.conn.SetReadDeadline(time.Time{})
		}

		frame, err := readFrame(s.reader, s.lim)
		if err != nil {
			s.failAll(err)
			return
		}
		s.touch()

		switch frame.MsgType {
		case MessageTypePong, MessageTypeHeartbeat:
			// Pong 也可能带 RequestID，走 pending 分发
			if frame.MsgType == MessageTypePong || frame.RequestID != 0 {
				s.dispatch(frame.RequestID, rpcResult{frame: frame})
			}
		case MessageTypeResponse:
			s.dispatch(frame.RequestID, rpcResult{frame: frame})
		case MessageTypePing:
			// 服务端不应向客户端发 Ping；若发来则回 Pong
			_ = s.writeFrame(NewFrame(MessageTypePong, frame.RequestID, nil, nil), time.Time{})
		default:
			// 忽略未知类型，避免拖垮连接
		}
	}
}

func (s *stream) dispatch(id uint64, res rpcResult) {
	s.mu.Lock()
	ch, ok := s.pending[id]
	if ok {
		delete(s.pending, id)
	}
	s.mu.Unlock()
	if !ok {
		return
	}
	select {
	case ch <- res:
	default:
	}
}

func (s *stream) failAll(err error) {
	if err == nil {
		err = errConnClosed
	}
	// 超时在健康连接上可能只是空闲读超时：若仍有 pending 则视为失败，否则安静关闭
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.closeErr = err
	pending := s.pending
	s.pending = make(map[uint64]chan rpcResult)
	s.mu.Unlock()

	_ = s.conn.Close()
	for id, ch := range pending {
		_ = id
		select {
		case ch <- rpcResult{err: err}:
		default:
		}
	}
}

func (s *stream) register(id uint64) (chan rpcResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		if s.closeErr != nil {
			return nil, s.closeErr
		}
		return nil, errConnClosed
	}
	if _, exists := s.pending[id]; exists {
		return nil, errors.New("client: duplicate request id")
	}
	ch := make(chan rpcResult, 1)
	s.pending[id] = ch
	return ch, nil
}

func (s *stream) unregister(id uint64) {
	s.mu.Lock()
	delete(s.pending, id)
	s.mu.Unlock()
}

// Call 在该 stream 上发送请求；非 OneWay 时等待匹配 RequestID 的响应。
func (s *stream) Call(ctx context.Context, req *Request) (*Response, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !s.Alive() {
		return nil, errConnClosed
	}

	s.inflight.Add(1)
	defer s.inflight.Add(-1)
	s.touch()

	frame := NewFrame(MessageTypeRequest, req.RequestID, req.Header, req.Body)
	if req.OneWay {
		frame.Flags |= FlagOneWay
	}

	var ch chan rpcResult
	if !req.OneWay {
		var err error
		ch, err = s.register(req.RequestID)
		if err != nil {
			return nil, err
		}
		defer s.unregister(req.RequestID)
	}

	deadline := writeDeadline(ctx, s.client.opts.writeTimeout)
	if err := s.writeFrame(frame, deadline); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		// 写失败：连接不可用
		s.failAll(err)
		return nil, err
	}

	if req.OneWay {
		return &Response{RequestID: req.RequestID}, nil
	}

	select {
	case <-ctx.Done():
		// 不关闭 stream：其他 in-flight 请求仍可用；响应到达后会被丢弃
		return nil, ctx.Err()
	case res := <-ch:
		if res.err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, res.err
		}
		if res.frame.IsError() {
			return nil, remoteErrorFromFrame(res.frame)
		}
		return &Response{
			RequestID: res.frame.RequestID,
			Header:    res.frame.Header,
			Body:      res.frame.Body,
		}, nil
	}
}

func (s *stream) writeFrame(f *Frame, deadline time.Time) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if !deadline.IsZero() {
		_ = s.conn.SetWriteDeadline(deadline)
	} else {
		_ = s.conn.SetWriteDeadline(time.Time{})
	}
	return writeFrame(s.conn, f, s.lim)
}

// Ping 发送应用层心跳并等待 Pong
func (s *stream) Ping(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	id := s.client.nextRequestID()
	ch, err := s.register(id)
	if err != nil {
		return err
	}
	defer s.unregister(id)

	frame := NewFrame(MessageTypePing, id, nil, nil)
	deadline := writeDeadline(ctx, s.client.opts.writeTimeout)
	if err := s.writeFrame(frame, deadline); err != nil {
		s.failAll(err)
		return err
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case res := <-ch:
		if res.err != nil {
			return res.err
		}
		if res.frame == nil || res.frame.MsgType != MessageTypePong {
			return errors.New("client: unexpected heartbeat response")
		}
		s.touch()
		return nil
	}
}

// Close 关闭 stream 并唤醒所有等待者
func (s *stream) Close() error {
	s.failAll(errConnClosed)
	return nil
}

func writeDeadline(ctx context.Context, fallback time.Duration) time.Time {
	deadline := time.Time{}
	if fallback > 0 {
		deadline = time.Now().Add(fallback)
	}
	if d, ok := ctx.Deadline(); ok {
		if deadline.IsZero() || d.Before(deadline) {
			deadline = d
		}
	}
	return deadline
}

