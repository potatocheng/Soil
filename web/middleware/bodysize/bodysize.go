package bodysize

import (
	"Soil/web"
	"net/http"
)

// MiddlewareBuilder 用于构建请求体大小限制中间件。
type MiddlewareBuilder struct {
	maxBytes int64
}

// Create 创建 MiddlewareBuilder，maxBytes 为允许的最大请求体字节数。
func Create(maxBytes int64) *MiddlewareBuilder {
	return &MiddlewareBuilder{
		maxBytes: maxBytes,
	}
}

// Build 构建请求体大小限制中间件。
//
// 策略：
//  1. 预先检查 ContentLength，若超过 maxBytes 则直接返回 413，不调用 next。
//  2. 同时使用 http.MaxBytesReader 包装 Body，防止 ContentLength 造假或 chunked
//     传输导致实际读取字节超限；此时 handler 在读 body 时会得到
//     "http: request body too large" 错误，由 handler 自行处理。
func (mb *MiddlewareBuilder) Build() web.Middleware {
	return func(next web.HandleFunc) web.HandleFunc {
		return func(ctx *web.Context) {
			if ctx.Req.ContentLength > mb.maxBytes {
				ctx.Abort(http.StatusRequestEntityTooLarge, "Request Entity Too Large")
				// Abort 将 ctx.done 置为 true。按 server.go 的设计，最外层中间件 m 在
				// next(ctx) 返回后仅在 !ctx.IsDone() 时调用 flashResp 写出响应，因此
				// 调用 Abort 后 flashResp 会被跳过。此处需直接写出响应，否则客户端会
				// 收到默认的 200 空响应。
				ctx.Resp.WriteHeader(http.StatusRequestEntityTooLarge)
				_, _ = ctx.Resp.Write([]byte("Request Entity Too Large"))
				return
			}
			ctx.Req.Body = http.MaxBytesReader(ctx.Resp, ctx.Req.Body, mb.maxBytes)
			next(ctx)
		}
	}
}
