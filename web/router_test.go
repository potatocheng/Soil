package web

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_Add_Route(t *testing.T) {
	mockHandler := func(ctx *Context) {}
	r := newRouter()

	assert.PanicsWithValue(t, "web: empty path", func() {
		r.addRoute(http.MethodGet, "", mockHandler)
	})

	assert.PanicsWithValue(t, "web: path must not end with '/'", func() {
		r.addRoute(http.MethodGet, "/abc/", mockHandler)
	})

	assert.PanicsWithValue(t, "web: path must begin with '/'", func() {
		r.addRoute(http.MethodGet, "abc/", mockHandler)
	})

	r.addRoute(http.MethodGet, "/", mockHandler)
	assert.PanicsWithValue(t, "web: root already has a handler", func() {
		r.addRoute(http.MethodGet, "/", mockHandler)
	})

	r.addRoute(http.MethodGet, "/a/b/c", mockHandler)
	assert.PanicsWithValue(t, "web: routing conflict", func() {
		r.addRoute(http.MethodGet, "/a/b/c", mockHandler)
	})

	path := "/a//b/c"
	expected := fmt.Sprintf("path[%s] invaild, 在路由中不能出现 //这种情况", path)
	assert.PanicsWithValue(t, expected, func() {
		r.addRoute(http.MethodGet, "/a//b/c", mockHandler)
	})

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

	if n.path != other.path {
		return fmt.Sprintf("两个节点路径不相同，分别是:[%s], [%s]", n.path, other.path), false
	}

	nHandleFunc := reflect.ValueOf(n.handler)
	otherHandleFunc := reflect.ValueOf(other.handler)
	if nHandleFunc != otherHandleFunc {
		return fmt.Sprintf("%s 节点 handler不相等 n %s, other %s", n.path, nHandleFunc.Type().String(), otherHandleFunc.Type().String()), false
	}

	if len(n.children) != len(other.children) {
		return fmt.Sprintf("%s 子节点长度不相同", n.path), false
	}

	if len(n.children) == 0 {
		return "", true
	}

	if n.starChild != nil {
		res, ok := n.starChild.equal(other.starChild)
		if !ok {
			return fmt.Sprintf("%s 通配符节点不匹配 %s", n.starChild.path, res), false
		}
	}

	if n.paramChild != nil {
		res, ok := n.paramChild.equal(other.paramChild)
		if !ok {
			return fmt.Sprintf("%s 路径参数节点不匹配 %s", n.paramChild.path, res), false
		}
	}

	if n.regexChild != nil {
		res, ok := n.regexChild.equal(other.regexChild)
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
	r := newRouter()
	mockHandler := func(ctx *Context) {}

	testRouter := []struct {
		method string
		path   string
	}{
		{
			method: http.MethodGet,
			path:   "/user/*",
		},
	}

	for _, route := range testRouter {
		r.addRoute(route.method, route.path, mockHandler)
	}

	testCases := []struct {
		testName      string
		method        string
		path          string
		want          bool
		wantMatchInfo *matchInfo
	}{
		{
			testName: "通配符匹配，通配符出现在中间",
			method:   http.MethodGet,
			path:     "/user/star/detail/china",
			want:     true,
			wantMatchInfo: &matchInfo{
				node:        &node{typ: nodeTypeAny, path: "*", handler: mockHandler},
				matchedPath: "/user/*",
			},
		},
	}

	for _, c := range testCases {
		t.Run(c.testName, func(t *testing.T) {
			mi, ok := r.findRoute(c.method, c.path)
			assert.Equal(t, c.want, ok)
			if !ok {
				return
			}

			assert.Equal(t, c.wantMatchInfo.paramPath, mi.paramPath)
			assert.Equal(t, c.wantMatchInfo.matchedPath, mi.matchedPath)
			assert.Equal(t, c.wantMatchInfo.node.path, mi.node.path)
			n := mi.node
			wantHandler := reflect.ValueOf(c.wantMatchInfo.node.handler)
			nVal := reflect.ValueOf(n.handler)
			assert.Equal(t, wantHandler, nVal)
		})
	}
}

func Test_find_router_path_param(t *testing.T) {
	r := newRouter()
	mockHandler := func(ctx *Context) {}

	r.addRoute(http.MethodGet, "/user/:id", mockHandler)

	testCases := []struct {
		testName      string
		method        string
		path          string
		want          bool
		wantParams    map[string]string
		wantMatchPath string
	}{
		{
			testName:      "路径参数匹配",
			method:        http.MethodGet,
			path:          "/user/123",
			want:          true,
			wantParams:    map[string]string{"id": "123"},
			wantMatchPath: "/user/:id",
		},
		{
			testName:      "路径参数匹配2",
			method:        http.MethodGet,
			path:          "/user/abc",
			want:          true,
			wantParams:    map[string]string{"id": "abc"},
			wantMatchPath: "/user/:id",
		},
	}

	for _, c := range testCases {
		t.Run(c.testName, func(t *testing.T) {
			mi, ok := r.findRoute(c.method, c.path)
			assert.Equal(t, c.want, ok)
			if !ok {
				return
			}
			assert.Equal(t, c.wantParams, mi.paramPath)
			assert.Equal(t, c.wantMatchPath, mi.matchedPath)
		})
	}
}

func Test_find_router_multi_path_param(t *testing.T) {
	r := newRouter()
	mockHandler := func(ctx *Context) {}

	r.addRoute(http.MethodGet, "/user/:id/post/:postId", mockHandler)

	testCases := []struct {
		testName      string
		method        string
		path          string
		want          bool
		wantParams    map[string]string
		wantMatchPath string
	}{
		{
			testName:      "多路径参数匹配",
			method:        http.MethodGet,
			path:          "/user/123/post/456",
			want:          true,
			wantParams:    map[string]string{"id": "123", "postId": "456"},
			wantMatchPath: "/user/:id/post/:postId",
		},
	}

	for _, c := range testCases {
		t.Run(c.testName, func(t *testing.T) {
			mi, ok := r.findRoute(c.method, c.path)
			assert.Equal(t, c.want, ok)
			if !ok {
				return
			}
			assert.Equal(t, c.wantParams, mi.paramPath)
			assert.Equal(t, c.wantMatchPath, mi.matchedPath)
		})
	}
}

func Test_find_router_regex(t *testing.T) {
	r := newRouter()
	mockHandler := func(ctx *Context) {}

	r.addRoute(http.MethodGet, "/detail/:id(^[0-9]+$)", mockHandler)

	testCases := []struct {
		testName      string
		method        string
		path          string
		want          bool
		wantMatchPath string
	}{
		{
			testName:      "正则匹配成功",
			method:        http.MethodGet,
			path:          "/detail/12345",
			want:          true,
			wantMatchPath: "/detail/:id(^[0-9]+$)",
		},
		{
			testName:      "正则匹配失败",
			method:        http.MethodGet,
			path:          "/detail/abc",
			want:          false,
			wantMatchPath: "",
		},
	}

	for _, c := range testCases {
		t.Run(c.testName, func(t *testing.T) {
			mi, ok := r.findRoute(c.method, c.path)
			assert.Equal(t, c.want, ok)
			if !ok {
				return
			}
			assert.Equal(t, c.wantMatchPath, mi.matchedPath)
		})
	}
}

func Test_router_concurrent(t *testing.T) {
	r := newRouter()
	wg := sync.WaitGroup{}
	numGoroutines := 100

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			r.addRoute(http.MethodGet, fmt.Sprintf("/user/%d", id), func(ctx *Context) {})
		}(i)
	}
	wg.Wait()

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			mi, ok := r.findRoute(http.MethodGet, fmt.Sprintf("/user/%d", id))
			assert.True(t, ok)
			assert.NotNil(t, mi.node)
			assert.NotNil(t, mi.node.handler)
		}(i)
	}
	wg.Wait()
}

// TestMethodNotAllowed_FindRoute TR-3.1
// 注册 GET /user/:id，POST /user/123 的 findRoute 返回 methodNotAllowed=true，allowedMethods 含 "GET"
func TestMethodNotAllowed_FindRoute(t *testing.T) {
	r := newRouter()
	mockHandler := func(ctx *Context) {}
	r.addRoute(http.MethodGet, "/user/:id", mockHandler)

	mi, ok := r.findRoute(http.MethodPost, "/user/123")
	assert.True(t, ok)
	assert.NotNil(t, mi)
	assert.True(t, mi.methodNotAllowed)
	assert.Contains(t, mi.allowedMethods, http.MethodGet)
}

// TestMethodNotAllowed_Serve TR-3.2
// serve 层面，注册 GET /user/:id，用 POST 请求 /user/123 返回 405，响应头 Allow 含 "GET"
func TestMethodNotAllowed_Serve(t *testing.T) {
	httpServer := NewHttpServer()
	httpServer.Get("/user/:id", func(ctx *Context) {
		ctx.RespString(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodPost, "/user/123", nil)
	w := httptest.NewRecorder()
	httpServer.ServeHTTP(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	assert.Equal(t, "405 method not allowed", w.Body.String())
	allow := w.Header().Get("Allow")
	assert.Contains(t, allow, http.MethodGet)
}

// TestMethodNotAllowed_MultipleMethods TR-3.3
// 注册 GET/PUT/DELETE /user/:id，POST /user/123 返回 405，Allow 含 "GET, PUT, DELETE"（顺序不限）
func TestMethodNotAllowed_MultipleMethods(t *testing.T) {
	httpServer := NewHttpServer()
	mockHandler := func(ctx *Context) {
		ctx.RespString(http.StatusOK, "ok")
	}
	httpServer.Get("/user/:id", mockHandler)
	httpServer.Put("/user/:id", mockHandler)
	httpServer.Delete("/user/:id", mockHandler)

	req := httptest.NewRequest(http.MethodPost, "/user/123", nil)
	w := httptest.NewRecorder()
	httpServer.ServeHTTP(w, req)

	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
	allow := w.Header().Get("Allow")
	assert.Contains(t, allow, http.MethodGet)
	assert.Contains(t, allow, http.MethodPut)
	assert.Contains(t, allow, http.MethodDelete)
}

// TestMethodNotAllowed_NotFound TR-3.4
// 完全不存在的路径 /notexist 返回 404 而非 405
func TestMethodNotAllowed_NotFound(t *testing.T) {
	httpServer := NewHttpServer()
	httpServer.Get("/user/:id", func(ctx *Context) {
		ctx.RespString(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/notexist", nil)
	w := httptest.NewRecorder()
	httpServer.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, "404 page not found", w.Body.String())
}
