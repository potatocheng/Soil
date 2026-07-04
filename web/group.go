package web

import (
	"net/http"
	"strings"
)

// RouterGroup 表示一组具有相同前缀和共享中间件的路由。
// 支持嵌套：子分组会继承父分组的前缀和中间件。
type RouterGroup struct {
	prefix string
	mdls   []Middleware
	server *HTTPServer
	parent *RouterGroup
}

// allMiddlewares 返回从根父分组到当前分组的所有中间件（父→子顺序）。
func (rg *RouterGroup) allMiddlewares() []Middleware {
	if rg.parent == nil {
		return rg.mdls
	}
	return append(rg.parent.allMiddlewares(), rg.mdls...)
}

// wrap 将分组中间件（父→子）依次包裹到 handler 外层，
// 最终执行顺序为：父中间件 → 子中间件 → handler。
func (rg *RouterGroup) wrap(handler HandleFunc) HandleFunc {
	allMdls := rg.allMiddlewares()
	h := handler
	for i := len(allMdls) - 1; i >= 0; i-- {
		h = allMdls[i](h)
	}
	return h
}

// fullPath 拼接当前分组（含所有祖先前缀）与给定 path 的完整路径。
// 保证结果以 "/" 开头，且不出现 "//"。
func (rg *RouterGroup) fullPath(path string) string {
	prefix := rg.prefix
	if rg.parent != nil {
		// 让父分组把自己的 prefix 拼到祖先前缀之上
		prefix = rg.parent.fullPath(rg.prefix)
	}
	if !strings.HasPrefix(prefix, "/") {
		prefix = "/" + prefix
	}
	prefix = strings.TrimSuffix(prefix, "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return prefix + path
}

// Use 向当前分组追加中间件。
func (rg *RouterGroup) Use(mdls ...Middleware) {
	if rg.mdls == nil {
		rg.mdls = mdls
		return
	}
	rg.mdls = append(rg.mdls, mdls...)
}

// Group 创建一个嵌套子分组，prefix 会拼接到当前分组前缀之后，
// 中间件会合并父分组的中间件。
func (rg *RouterGroup) Group(prefix string, mdls ...Middleware) *RouterGroup {
	return &RouterGroup{
		prefix: prefix,
		mdls:   mdls,
		server: rg.server,
		parent: rg,
	}
}

func (rg *RouterGroup) Get(path string, handler HandleFunc) {
	rg.server.addRoute(http.MethodGet, rg.fullPath(path), rg.wrap(handler))
}

func (rg *RouterGroup) Post(path string, handler HandleFunc) {
	rg.server.addRoute(http.MethodPost, rg.fullPath(path), rg.wrap(handler))
}

func (rg *RouterGroup) Put(path string, handler HandleFunc) {
	rg.server.addRoute(http.MethodPut, rg.fullPath(path), rg.wrap(handler))
}

func (rg *RouterGroup) Delete(path string, handler HandleFunc) {
	rg.server.addRoute(http.MethodDelete, rg.fullPath(path), rg.wrap(handler))
}

func (rg *RouterGroup) Patch(path string, handler HandleFunc) {
	rg.server.addRoute(http.MethodPatch, rg.fullPath(path), rg.wrap(handler))
}

func (rg *RouterGroup) Options(path string, handler HandleFunc) {
	rg.server.addRoute(http.MethodOptions, rg.fullPath(path), rg.wrap(handler))
}

func (rg *RouterGroup) Head(path string, handler HandleFunc) {
	rg.server.addRoute(http.MethodHead, rg.fullPath(path), rg.wrap(handler))
}
