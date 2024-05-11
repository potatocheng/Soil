package web

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"net/http"
	"reflect"
	"testing"
)

func Test_Add_Route(t *testing.T) {
	mockHandler := func(ctx *Context) {}
	r := newRouter()

	//测试非法用例
	assert.PanicsWithValue(t, "web: empty path", func() {
		r.addRoute(http.MethodGet, "", mockHandler)
	})

	assert.PanicsWithValue(t, "web: path must not end with '/'", func() {
		r.addRoute(http.MethodGet, "/abc/", mockHandler)
	})

	assert.PanicsWithValue(t, "web: path must begin with '/'", func() {
		r.addRoute(http.MethodGet, "abc/", mockHandler)
	})

	//重复注册根节点
	r.addRoute(http.MethodGet, "/", mockHandler)
	assert.PanicsWithValue(t, "web: root already has a handler", func() {
		r.addRoute(http.MethodGet, "/", mockHandler)
	})

	//重复注册其他节点
	r.addRoute(http.MethodGet, "/a/b/c", mockHandler)
	assert.PanicsWithValue(t, "web: routing conflict", func() {
		r.addRoute(http.MethodGet, "/a/b/c", mockHandler)
	})

	//路由中出现//情况，如/a//b/c
	path := "/a//b/c"
	expected := fmt.Sprintf("path[%s] invaild, 在路由中不能出现 //这种情况", path)
	assert.PanicsWithValue(t, expected, func() {
		r.addRoute(http.MethodGet, "/a//b/c", mockHandler)
	})

	//测试路径参数，通配符和正则路由的互斥
	path = "/user/:id"
	r.addRoute(http.MethodGet, "/user/:id", mockHandler)
	expected = fmt.Sprintf("web: 非法路由，已有路径参数路由。不允许同时注册通配符路由和参数路由 [%s]", "*")
	assert.PanicsWithValue(t, expected, func() {
		r.addRoute(http.MethodGet, "/user/*", mockHandler)
	})

	assert.PanicsWithValue(t, "web: 非法路由，已有正则路由。不允许同时注册通配符路由和正则路由 [*]", func() {
		r.addRoute(http.MethodGet, "/login/:id(.*)", mockHandler)
		r.addRoute(http.MethodGet, "/login/*", mockHandler)
	})

	r = newRouter()
	assert.PanicsWithValue(t, "web: 非法路由，已有通配符路由。不允许同时注册通配符路由和参数路由 [:id]", func() {
		r.addRoute(http.MethodGet, "/user/*", mockHandler)
		r.addRoute(http.MethodGet, "/user/:id", mockHandler)
	})

	assert.PanicsWithValue(t, "web: 非法路由，已有正则路由。不允许同时注册通配符路由和正则路由 [:id]", func() {
		r.addRoute(http.MethodPost, "/user/:id(.*)", mockHandler)
		r.addRoute(http.MethodPost, "/user/:id", mockHandler)
	})

	r = newRouter()
	assert.PanicsWithValue(t, "web: 非法路由，已有参数路由。不允许同时注册正则路由和参数路由 [:id(.*)]", func() {
		r.addRoute(http.MethodGet, "/user/:id", mockHandler)
		r.addRoute(http.MethodGet, "/user/:id(.*)", mockHandler)
	})

	assert.PanicsWithValue(t, "web: 非法路由，已有通配符路由。不允许同时注册正则路由和通配符路由 [:id(.*)]", func() {
		r.addRoute(http.MethodPost, "/user/*", mockHandler)
		r.addRoute(http.MethodPost, "/user/:id(.*)", mockHandler)
	})

	r = newRouter()
	//测试路由的插入是否符合预期
	testRoutes := []struct {
		method string
		path   string
	}{
		{
			method: http.MethodGet,
			path:   "/",
		},
		{
			method: http.MethodGet,
			path:   "/hello",
		},
		{
			method: http.MethodPost,
			path:   "/hello/:id",
		},
		{
			method: http.MethodPost,
			path:   "/login/*/user",
		},
		{
			method: http.MethodPost,
			path:   "/detail/:id(^[0-9]+$)",
		},
	}

	for _, route := range testRoutes {
		r.addRoute(route.method, route.path, mockHandler)
	}

	wantRouter := &router{
		trees: map[string]*node{
			http.MethodGet: &node{
				path: "/",
				children: map[string]*node{
					"hello": &node{path: "hello", handler: mockHandler},
				},
				handler: mockHandler,
			},
			http.MethodPost: &node{
				path: "/",
				children: map[string]*node{
					"hello":  &node{path: "hello", paramChild: &node{path: ":id", handler: mockHandler}},
					"login":  &node{path: "login", starChild: &node{path: "*", starChild: &node{path: "user", handler: mockHandler}}},
					"detail": &node{path: "detail", regexChild: &node{path: ":id(^[0-9]+$)", handler: mockHandler}},
				},
			},
		},
	}
	msg, ok := r.equal(wantRouter)
	assert.True(t, ok, msg)
}

func (r *router) equal(other *router) (string, bool) {
	for k, v := range r.trees {
		yv, ok := other.trees[k]
		if !ok {
			return fmt.Sprintf("目标 router 里面没有方法 %s 的路由树", k), false
		}
		str, ok := v.equal(yv)
		if !ok {
			return k + "-" + str, ok
		}
	}

	return "", true
}

func (n *node) equal(other *node) (string, bool) {
	if other == nil {
		return "目标节点为nil", false
	}

	//判断节点路径是否相同
	if n.path != other.path {
		return fmt.Sprintf("两个节点路径不相同，分别是:[%s], [%s]", n.path, other.path), false
	}

	//判断节点回调函数是否相同
	nHandleFunc := reflect.ValueOf(n.handler)
	otherHandleFunc := reflect.ValueOf(other.handler)
	if nHandleFunc != otherHandleFunc {
		return fmt.Sprintf("%s 节点 handler不相等 n %s, other %s", n.path, nHandleFunc.Type().String(), otherHandleFunc.Type().String()), false
	}

	//判断静态节点是否匹配
	if len(n.children) != len(other.children) {
		return fmt.Sprintf("%s 子节点长度不相同", n.path), false
	}

	//遍历到叶节点直接返回
	if len(n.children) == 0 {
		return "", true
	}

	//判断通配符子节点是否匹配
	if n.starChild != nil {
		res, ok := n.starChild.equal(other.starChild)
		if !ok {
			return fmt.Sprintf("%s 通配符节点不匹配 %s", n.starChild.path, res), false
		}
	}

	//判断路径参数是否匹配
	if n.paramChild != nil {
		res, ok := n.paramChild.equal(other.paramChild)
		if !ok {
			return fmt.Sprintf("%s 路径参数节点不匹配 %s", n.paramChild.path, res), false
		}
	}

	//判断正则子节点是否匹配
	if n.regexChild != nil {
		res, ok := n.regexChild.equal(other.paramChild)
		if !ok {
			return fmt.Sprintf("%s 正则节点不匹配 %s", n.regexChild.path, res), false
		}
	}

	for nPath, nNode := range n.children {
		otherNode, ok := other.children[nPath]
		if !ok {
			return fmt.Sprintf("%s 目标节点缺少子节点 %s", n.path, nPath), false
		}

		res, ok := nNode.equal(otherNode)
		if !ok {
			return n.path + "-" + res, false
		}
	}

	return "", true
}

func Test_find_router(t *testing.T) {

}
