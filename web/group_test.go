package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TR-11.1: server.Group("/api/v1").Get("/users", h) 注册后，
// 请求 /api/v1/users 命中 h 返回 200。
func TestGroup_BasicRoute(t *testing.T) {
	httpServer := NewHttpServer()

	g := httpServer.Group("/api/v1")
	g.Get("/users", func(ctx *Context) {
		ctx.RespString(http.StatusOK, "users")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	w := httptest.NewRecorder()
	httpServer.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "users", w.Body.String())
}

// TR-11.2: 分组带中间件，中间件被执行（通过响应头标记验证）。
func TestGroup_Middleware(t *testing.T) {
	httpServer := NewHttpServer()

	mw := func(next HandleFunc) HandleFunc {
		return func(ctx *Context) {
			ctx.Resp.Header().Set("X-Group-Mw", "yes")
			next(ctx)
		}
	}

	g := httpServer.Group("/api", mw)
	g.Get("/ping", func(ctx *Context) {
		ctx.RespString(http.StatusOK, "pong")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/ping", nil)
	w := httptest.NewRecorder()
	httpServer.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "pong", w.Body.String())
	assert.Equal(t, "yes", w.Header().Get("X-Group-Mw"))
}

// TR-11.3: 嵌套分组 g1 := server.Group("/api"); g2 := g1.Group("/v2");
// g2.Get("/items", h) 注册后，请求 /api/v2/items 命中。
func TestGroup_Nested(t *testing.T) {
	httpServer := NewHttpServer()

	g1 := httpServer.Group("/api")
	g2 := g1.Group("/v2")
	g2.Get("/items", func(ctx *Context) {
		ctx.RespString(http.StatusOK, "items")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v2/items", nil)
	w := httptest.NewRecorder()
	httpServer.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "items", w.Body.String())
}

// TR-11.4: 全局中间件 + 分组中间件执行顺序（全局先执行，分组后执行）。
// 通过向 X-Order 头追加标记验证顺序。
func TestGroup_GlobalAndGroupMiddlewareOrder(t *testing.T) {
	httpServer := NewHttpServer()

	// 全局中间件：最先执行，追加 "global"
	httpServer.Use(func(next HandleFunc) HandleFunc {
		return func(ctx *Context) {
			ctx.Resp.Header().Add("X-Order", "global")
			next(ctx)
		}
	})

	// 分组中间件：在全局之后执行，追加 "group"
	g := httpServer.Group("/api", func(next HandleFunc) HandleFunc {
		return func(ctx *Context) {
			ctx.Resp.Header().Add("X-Order", "group")
			next(ctx)
		}
	})
	g.Get("/order", func(ctx *Context) {
		ctx.Resp.Header().Add("X-Order", "handler")
		ctx.RespString(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/order", nil)
	w := httptest.NewRecorder()
	httpServer.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	// 顺序应为：global -> group -> handler
	assert.Equal(t, []string{"global", "group", "handler"}, w.Header().Values("X-Order"))
}

// 补充：嵌套分组中间件合并（父分组中间件先于子分组中间件执行）。
func TestGroup_NestedMiddleware(t *testing.T) {
	httpServer := NewHttpServer()

	g1 := httpServer.Group("/api", func(next HandleFunc) HandleFunc {
		return func(ctx *Context) {
			ctx.Resp.Header().Add("X-Order", "parent")
			next(ctx)
		}
	})
	g2 := g1.Group("/v2", func(next HandleFunc) HandleFunc {
		return func(ctx *Context) {
			ctx.Resp.Header().Add("X-Order", "child")
			next(ctx)
		}
	})
	g2.Get("/x", func(ctx *Context) {
		ctx.RespString(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v2/x", nil)
	w := httptest.NewRecorder()
	httpServer.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, []string{"parent", "child"}, w.Header().Values("X-Order"))
}

// 补充：分组的多种 HTTP 方法都能正确注册和命中。
func TestGroup_MultipleMethods(t *testing.T) {
	httpServer := NewHttpServer()
	g := httpServer.Group("/api")

	g.Post("/p", func(ctx *Context) { ctx.RespString(http.StatusOK, "post") })
	g.Put("/p", func(ctx *Context) { ctx.RespString(http.StatusOK, "put") })
	g.Delete("/p", func(ctx *Context) { ctx.RespString(http.StatusOK, "delete") })
	g.Patch("/p", func(ctx *Context) { ctx.RespString(http.StatusOK, "patch") })
	g.Options("/p", func(ctx *Context) { ctx.RespString(http.StatusOK, "options") })
	g.Head("/p", func(ctx *Context) { ctx.RespString(http.StatusOK, "head") })

	cases := []struct {
		method string
		want   string
	}{
		{http.MethodPost, "post"},
		{http.MethodPut, "put"},
		{http.MethodDelete, "delete"},
		{http.MethodPatch, "patch"},
		{http.MethodOptions, "options"},
		{http.MethodHead, "head"},
	}
	for _, c := range cases {
		t.Run(c.method, func(t *testing.T) {
			req := httptest.NewRequest(c.method, "/api/p", nil)
			w := httptest.NewRecorder()
			httpServer.ServeHTTP(w, req)
			assert.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, c.want, w.Body.String())
		})
	}
}
