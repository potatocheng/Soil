package accesslog

import (
	"Soil/web"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

type accessLog struct {
	Host       string `json:"host"`
	Route      string `json:"route"`
	HTTPMethod string `json:"http_method"`
	Path       string `json:"path"`
	Status     int    `json:"status"`
	LatencyMs  int64  `json:"latency_ms"`
	RespSize   int    `json:"resp_size"`
	ClientIP   string `json:"client_ip"`
	UserAgent  string `json:"user_agent"`
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
			start := time.Now()
			defer func() {
				status := ctx.RespStatusCode
				if status == 0 {
					status = 200
				}
				l := accessLog{
					Host:       ctx.Req.Host,
					Route:      ctx.MatchedRoute,
					Path:       ctx.Req.URL.Path,
					HTTPMethod: ctx.Req.Method,
					Status:     status,
					LatencyMs:  time.Since(start).Milliseconds(),
					RespSize:   len(ctx.RespData),
					ClientIP:   getClientIP(ctx.Req),
					UserAgent:  ctx.Req.UserAgent(),
				}
				val, _ := json.Marshal(l)
				mb.logFunc(string(val))
			}()
			next(ctx)
		}
	}
}

// getClientIP 从请求中解析客户端 IP。
// 优先取 X-Forwarded-For 第一个 IP，回退 X-Real-IP，再回退 RemoteAddr 去端口。
func getClientIP(req *http.Request) string {
	// X-Forwarded-For: client, proxy1, proxy2
	if xff := req.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.SplitN(xff, ",", 2)
		ip := strings.TrimSpace(parts[0])
		if ip != "" {
			return ip
		}
	}
	// X-Real-IP
	if xri := req.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	// RemoteAddr，去除端口
	host, _, err := net.SplitHostPort(req.RemoteAddr)
	if err != nil {
		return req.RemoteAddr
	}
	return host
}
