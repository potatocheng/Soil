package net

import "context"

// Request 表示一个请求
type Request struct {
	Ctx       context.Context
	RequestID uint64
	Header    []byte
	Body      []byte
	// OneWay 为 true 时客户端不等待响应，服务端不写回（通过 FlagOneWay 传递）
	OneWay bool
}

// Response 表示一个响应
type Response struct {
	RequestID uint64
	Header    []byte
	Body      []byte
	Error     error
}

// Handler 处理请求
type Handler interface {
	Handle(ctx context.Context, req *Request) (*Response, error)
}

// HandlerFunc 允许普通函数作为 Handler
type HandlerFunc func(ctx context.Context, req *Request) (*Response, error)

// Handle 实现 Handler 接口
func (f HandlerFunc) Handle(ctx context.Context, req *Request) (*Response, error) {
	return f(ctx, req)
}

// Middleware 以洋葱模型包装 Handler
type Middleware func(Handler) Handler

// Chain 按注册顺序从外到内组合中间件
func Chain(h Handler, mws ...Middleware) Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

// EchoHandler 是默认的 echo 处理器，仅用于测试和示例
var EchoHandler Handler = HandlerFunc(func(ctx context.Context, req *Request) (*Response, error) {
	return &Response{
		RequestID: req.RequestID,
		Header:    req.Header,
		Body:      req.Body,
	}, nil
})
