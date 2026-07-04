package requestid

import (
	"Soil/web"

	"github.com/google/uuid"
)

// defaultHeaderName 是 RequestID 默认使用的请求/响应头名称。
const defaultHeaderName = "X-Request-Id"

// MiddlewareBuilder 用于链式配置并构建 RequestID 中间件。
type MiddlewareBuilder struct {
	headerName string
}

// Create 返回一个带有默认配置的 MiddlewareBuilder（headerName 默认为 "X-Request-Id"）。
func Create() *MiddlewareBuilder {
	return &MiddlewareBuilder{
		headerName: defaultHeaderName,
	}
}

// WithHeaderName 设置用于读取请求头与写入响应头的名称，默认为 "X-Request-Id"。
func (mb *MiddlewareBuilder) WithHeaderName(name string) *MiddlewareBuilder {
	mb.headerName = name
	return mb
}

// Build 构建并返回 RequestID 中间件。
//
// 中间件行为：
//   - 优先沿用请求头中的 RequestID（用 mb.headerName 读取）；
//   - 若请求头为空，则用 uuid.New().String() 生成新的 UUID v4；
//   - 将 requestID 写入 ctx.RequestID，供后续 handler 读取；
//   - 通过 ctx.SetHeader 写入 RespHeaders（由框架的 flashResp 统一写出），
//     同时直接写入 ctx.Resp.Header() 作为双保险，确保即便后续中间件调用
//     Abort 导致 flashResp 被跳过时，响应头仍能正确写出。
func (mb *MiddlewareBuilder) Build() web.Middleware {
	return func(next web.HandleFunc) web.HandleFunc {
		return func(ctx *web.Context) {
			requestID := ctx.Req.Header.Get(mb.headerName)
			if requestID == "" {
				requestID = uuid.New().String()
			}

			ctx.RequestID = requestID

			// 通过 SetHeader 写入 RespHeaders，由 flashResp 统一写出。
			ctx.SetHeader(mb.headerName, requestID)
			// 双保险：直接写入 ResponseWriter，确保 Abort 场景下响应头仍能写出。
			ctx.Resp.Header().Set(mb.headerName, requestID)

			next(ctx)
		}
	}
}
