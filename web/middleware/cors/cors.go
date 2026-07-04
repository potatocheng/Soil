package cors

import (
	"Soil/web"
	"net/http"
	"strconv"
	"strings"
)

// defaultMaxAge 是预检请求结果的默认缓存时长（12 小时，单位秒）。
const defaultMaxAge = 12 * 60 * 60

// MiddlewareBuilder 用于链式配置并构建 CORS 中间件。
type MiddlewareBuilder struct {
	allowOrigins     []string
	allowMethods     []string
	allowHeaders     []string
	exposeHeaders    []string
	allowCredentials bool
	maxAge           int
}

// Create 返回一个带有合理默认值的 MiddlewareBuilder。
// 默认允许的方法为 GET/POST/PUT/DELETE/PATCH/OPTIONS/HEAD，默认 MaxAge 为 12 小时。
// 默认不允许任何 Origin，需通过 AllowOrigins 显式配置。
func Create() *MiddlewareBuilder {
	return &MiddlewareBuilder{
		allowMethods: []string{
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodDelete,
			http.MethodPatch,
			http.MethodOptions,
			http.MethodHead,
		},
		maxAge: defaultMaxAge,
	}
}

// AllowOrigins 设置允许的 Origin 列表，"*" 表示允许任意 Origin。
func (mb *MiddlewareBuilder) AllowOrigins(origins ...string) *MiddlewareBuilder {
	mb.allowOrigins = origins
	return mb
}

// AllowMethods 设置允许的 HTTP 方法。覆盖默认值。
func (mb *MiddlewareBuilder) AllowMethods(methods ...string) *MiddlewareBuilder {
	mb.allowMethods = methods
	return mb
}

// AllowHeaders 设置允许的请求头。
func (mb *MiddlewareBuilder) AllowHeaders(headers ...string) *MiddlewareBuilder {
	mb.allowHeaders = headers
	return mb
}

// ExposeHeaders 设置允许前端读取的响应头。
func (mb *MiddlewareBuilder) ExposeHeaders(headers ...string) *MiddlewareBuilder {
	mb.exposeHeaders = headers
	return mb
}

// AllowCredentials 设置是否允许携带凭证（Cookie 等）。
func (mb *MiddlewareBuilder) AllowCredentials(b bool) *MiddlewareBuilder {
	mb.allowCredentials = b
	return mb
}

// MaxAge 设置预检请求结果的缓存时长（秒）。
func (mb *MiddlewareBuilder) MaxAge(seconds int) *MiddlewareBuilder {
	mb.maxAge = seconds
	return mb
}

// hasWildcardOrigin 判断允许的 Origin 列表中是否包含通配符 "*"。
func (mb *MiddlewareBuilder) hasWildcardOrigin() bool {
	for _, o := range mb.allowOrigins {
		if o == "*" {
			return true
		}
	}
	return false
}

// matchOrigin 返回应当写入 Access-Control-Allow-Origin 的值。
// 若配置了 "*"，则对所有非空 Origin 返回 "*"；
// 否则做精确匹配，命中则返回该 Origin，未命中或 Origin 为空则返回空字符串。
func (mb *MiddlewareBuilder) matchOrigin(origin string) string {
	if origin == "" {
		return ""
	}
	for _, o := range mb.allowOrigins {
		if o == "*" {
			return "*"
		}
		if o == origin {
			return origin
		}
	}
	return ""
}

// Build 构建并返回 CORS 中间件。
// 若同时配置了 AllowOrigins("*") 与 AllowCredentials(true)，则 panic（W3C 规范互斥）。
func (mb *MiddlewareBuilder) Build() web.Middleware {
	if mb.hasWildcardOrigin() && mb.allowCredentials {
		panic("cors: AllowOrigins(\"*\") 与 AllowCredentials(true) 互斥，违反 W3C CORS 规范")
	}

	return func(next web.HandleFunc) web.HandleFunc {
		return func(ctx *web.Context) {
			origin := ctx.Req.Header.Get("Origin")
			allowOrigin := mb.matchOrigin(origin)

			// 处理 OPTIONS 预检请求。
			if ctx.Req.Method == http.MethodOptions {
				// ctx.Abort 会将请求标记为 done，框架的 m 中间件因此会跳过
				// flashResp，所以这里必须直接把响应头与状态码写到 ResponseWriter。
				if allowOrigin != "" {
					header := ctx.Resp.Header()
					header.Set("Access-Control-Allow-Origin", allowOrigin)
					if mb.allowCredentials {
						header.Set("Access-Control-Allow-Credentials", "true")
					}
					if len(mb.allowMethods) > 0 {
						header.Set("Access-Control-Allow-Methods", strings.Join(mb.allowMethods, ", "))
					}
					if len(mb.allowHeaders) > 0 {
						header.Set("Access-Control-Allow-Headers", strings.Join(mb.allowHeaders, ", "))
					}
					if mb.maxAge > 0 {
						header.Set("Access-Control-Max-Age", strconv.Itoa(mb.maxAge))
					}
					header.Add("Vary", "Origin")
				}
				ctx.Resp.WriteHeader(http.StatusNoContent)
				ctx.Abort(http.StatusNoContent, "")
				return
			}

			// 处理普通请求：通过 SetHeader 追加 CORS 响应头，
			// 由框架的 flashResp 统一写出。
			if allowOrigin != "" {
				ctx.SetHeader("Access-Control-Allow-Origin", allowOrigin)
				if mb.allowCredentials {
					ctx.SetHeader("Access-Control-Allow-Credentials", "true")
				}
				if len(mb.exposeHeaders) > 0 {
					ctx.SetHeader("Access-Control-Expose-Headers", strings.Join(mb.exposeHeaders, ", "))
				}
				ctx.SetHeader("Vary", "Origin")
			}
			next(ctx)
		}
	}
}
