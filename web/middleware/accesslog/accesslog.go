package accesslog

import (
	"Soil/web"
	"encoding/json"
	"log"
)

type accessLog struct {
	Host       string
	Route      string
	HTTPMethod string
	Path       string
}

type MiddlewareBuilder struct {
	logFunc func(accessLog string)
}

func (mb *MiddlewareBuilder) WithLogFunc(logFunc func(accessLog string)) *MiddlewareBuilder {
	mb.logFunc = logFunc
	return mb
}

func Create() *MiddlewareBuilder {
	return &MiddlewareBuilder{
		logFunc: func(accessLog string) {
			log.Println(accessLog)
		},
	}
}

func (mb *MiddlewareBuilder) Build() web.Middleware {
	return func(next web.HandleFunc) web.HandleFunc {
		return func(ctx *web.Context) {
			defer func() {
				l := accessLog{
					Host:       ctx.Req.Host,
					Route:      ctx.MatchedRoute,
					Path:       ctx.Req.URL.Path,
					HTTPMethod: ctx.Req.Method,
				}
				val, _ := json.Marshal(l)
				mb.logFunc(string(val))
			}()
			next(ctx)
		}
	}
}
