package timeout

import (
	"Soil/web"
	"context"
	"log"
	"net/http"
	"time"
)

// MiddlewareBuilder 用于构建请求超时中间件。
type MiddlewareBuilder struct {
	timeout time.Duration
}

// Create 创建一个带有指定超时时长的 MiddlewareBuilder。
func Create(timeout time.Duration) *MiddlewareBuilder {
	return &MiddlewareBuilder{
		timeout: timeout,
	}
}

// Build 构建超时中间件。
//
// handler 在独立 goroutine 中执行，主流程通过 select 监听 panicCh / done /
// timeoutCtx.Done() 三个分支：
//   - panicCh：handler 发生 panic。panic 在子 goroutine 中产生，外层 recovery
//     中间件（位于主 goroutine）的 defer recover() 无法跨 goroutine 捕获，
//     因此由 timeout 中间件在 goroutine 内部自行 recover，并通过 panicCh 传回
//     主 goroutine 设置 500 状态码（模仿 recovery 中间件行为），交由上层框架
//     flashResp 写回响应。若不在此处捕获，panic 会崩溃整个进程（exit code 2）。
//   - done：handler 正常返回。若此时 context 已超时（与 done 同时就绪，handler
//     因 context 取消而退出），按超时处理返回 503；否则交由上层框架 flashResp
//     写回 handler 设置的响应。
//   - timeoutCtx.Done()：超时先到，返回 503。
//
// 注意：所有分支均只设置 ctx.RespStatusCode / ctx.RespData，不调用 ctx.Abort
// （Abort 会将 ctx.done 置为 true，导致上层框架 m 中间件跳过 flashResp），
// 由 flashResp 统一写回响应，行为更一致。
//
// goroutine 安全：超时分支不再读取 handler goroutine 的结果；当 done 与
// timeoutCtx.Done() 同时就绪时（handler 因 context 取消而退出），通过检查
// timeoutCtx.Err() 统一按超时处理，避免 select 随机选中 done 分支导致空响应。
// panic 分支在 recover 后不 close(done)，确保主流程优先走 panicCh 分支，
// 避免 panic 与 done 同时就绪时的竞态。panicCh 缓冲为 1，即使主流程因超时
// 先返回不再读取 panicCh，goroutine 发送也不会阻塞，避免 goroutine 泄漏。
func (mb *MiddlewareBuilder) Build() web.Middleware {
	return func(next web.HandleFunc) web.HandleFunc {
		return func(ctx *web.Context) {
			timeoutCtx, cancel := context.WithTimeout(ctx.Req.Context(), mb.timeout)
			defer cancel()

			// 使后续 handler 能感知到 context 取消
			ctx.Req = ctx.Req.WithContext(timeoutCtx)

			done := make(chan struct{})
			// 缓冲为 1 避免 goroutine 泄漏：即使主流程因超时先返回不再读取
			// panicCh，goroutine 向 panicCh 发送也不会阻塞。
			panicCh := make(chan any, 1)

			go func() {
				defer func() {
					if r := recover(); r != nil {
						// 跨 goroutine 的 panic 无法被外层 recovery 捕获，
						// 在此 recover 并通过 panicCh 传回主 goroutine。
						panicCh <- r
						// 不 close(done)，让主流程走 panicCh 分支，
						// 避免 panic 与 done 同时就绪时 select 随机选中 done。
						return
					}
					close(done)
				}()
				next(ctx)
			}()

			select {
			case r := <-panicCh:
				// handler panic：设置 500（模仿 recovery 中间件行为），
				// 由 flashResp 统一写出。ctx.done 仍为 false，flashResp 会执行。
				log.Printf("timeout middleware: panic recovered from handler goroutine: %v", r)
				ctx.RespStatusCode = http.StatusInternalServerError
				ctx.RespData = []byte("Internal Server Error")
			case <-done:
				// handler 已返回。若此时 context 已超时（与 done 同时就绪），
				// 按超时处理；否则什么都不做，交由上层框架 flashResp 写回
				// handler 设置的响应。
				if timeoutCtx.Err() != nil {
					ctx.RespStatusCode = http.StatusServiceUnavailable
					ctx.RespData = []byte("Service Unavailable")
				}
			case <-timeoutCtx.Done():
				ctx.RespStatusCode = http.StatusServiceUnavailable
				ctx.RespData = []byte("Service Unavailable")
			}
		}
	}
}
