package recovery

import (
	"Soil/web"
	"log"
	"net/http"
)

type MiddlewareBuilder struct {
	LogFunc func(err any)
}

func Create() *MiddlewareBuilder {
	return &MiddlewareBuilder{
		LogFunc: func(err any) {
			log.Printf("panic recovered: %v", err)
		},
	}
}

func (mb *MiddlewareBuilder) WithLogFunc(logFunc func(err any)) *MiddlewareBuilder {
	mb.LogFunc = logFunc
	return mb
}

func (mb *MiddlewareBuilder) Build() web.Middleware {
	return func(next web.HandleFunc) web.HandleFunc {
		return func(ctx *web.Context) {
			defer func() {
				if r := recover(); r != nil {
					mb.LogFunc(r)
					ctx.RespStatusCode = http.StatusInternalServerError
					ctx.RespData = []byte("Internal Server Error")
				}
			}()
			next(ctx)
		}
	}
}