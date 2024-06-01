package errhdl

import "Soil/web"

type MiddlewareBuilder struct {
	resp map[int][]byte
}

func NewMiddlewareBuilder() *MiddlewareBuilder {
	return &MiddlewareBuilder{resp: make(map[int][]byte, 64)}
}

// RegisterError 注册一个错误码，错误发生时，返回特定的错误数据(字符串或者页面)；
// 可以用来解决错误页面重定向的问题
func (m *MiddlewareBuilder) RegisterError(code int, resp []byte) *MiddlewareBuilder {
	m.resp[code] = resp

	return m
}

func (m *MiddlewareBuilder) Build() web.Middleware {
	return func(next web.HandleFunc) web.HandleFunc {
		return func(ctx *web.Context) {
			next(ctx)
			resp, ok := m.resp[ctx.RespStatusCode]
			if ok {
				ctx.RespData = resp
			}
		}
	}
}
