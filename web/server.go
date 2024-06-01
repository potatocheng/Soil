package web

import (
	"log"
	"net/http"
)

type HandleFunc func(ctx *Context)

type Server interface {
	// Handler interface {ServeHTTP(ResponseWriter, *Request)}
	http.Handler

	// Start 启动服务器;
	// addr是监听地址。包括ip:port
	Start(addr string) error

	// addRoute 注册路由
	// method 是HTTP方法， path是路由，handler是路由被命中时执行的回调
	addRoute(method string, path string, handler HandleFunc)
}

// 确保HttpServer实现了所有 Server 接口
var _ Server = &HTTPServer{}

type HTTPServer struct {
	router
	mdls []Middleware
}

func NewHttpServer() *HTTPServer {
	return &HTTPServer{
		router: newRouter(),
	}
}

func (hs *HTTPServer) Use(mdls ...Middleware) {
	if hs.mdls == nil {
		hs.mdls = mdls
		return
	}

	hs.mdls = append(hs.mdls, mdls...)
}

func (hs *HTTPServer) Start(addr string) error {
	return http.ListenAndServe(addr, hs)
}

func (hs *HTTPServer) flashResp(ctx *Context) {
	if ctx.RespStatusCode > 0 {
		ctx.Resp.WriteHeader(ctx.RespStatusCode)
	}
	_, err := ctx.Resp.Write(ctx.RespData)
	if err != nil {
		log.Fatalln("回写响应失败", err)
	}
}

// 实现http.Handler接口，这样http请求到来时会触发这个函数
func (hs *HTTPServer) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	ctx := &Context{
		Req:  request,
		Resp: response,
	}

	//将Use函数保存得到的函数从后往前组装(责任链模式)，执行时就是从前往后执行
	root := hs.serve
	for i := len(hs.mdls) - 1; i >= 0; i-- {
		root = hs.mdls[i](root)
	}

	var m Middleware = func(next HandleFunc) HandleFunc {
		return func(ctx *Context) {
			next(ctx)
			hs.flashResp(ctx)
		}
	}

	root = m(root)
	root(ctx)
}

func (hs *HTTPServer) serve(ctx *Context) {
	mi, ok := hs.findRoute(ctx.Req.Method, ctx.Req.URL.Path)
	if !ok || mi.node == nil || mi.node.handler == nil {
		ctx.Resp.WriteHeader(http.StatusNotFound)
		ctx.Resp.Write([]byte("404 page not found"))
		return
	}
	ctx.PathParams = mi.paramPath
	mi.node.handler(ctx)
}

func (hs *HTTPServer) Post(path string, handler HandleFunc) {
	hs.addRoute(http.MethodPost, path, handler)
}

func (hs *HTTPServer) Get(path string, handler HandleFunc) {
	hs.addRoute(http.MethodGet, path, handler)
}
