package web

import "net/http"

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
}

func NewHttpServer() *HTTPServer {
	return &HTTPServer{}
}

func (hs *HTTPServer) Start(addr string) error {
	return http.ListenAndServe(addr, hs)
}

// 实现http.Handler接口，这样http请求到来时会触发这个函数
func (hs *HTTPServer) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	ctx := &Context{
		Req:  request,
		Resp: response,
	}

	hs.serve(ctx)
}

func (hs *HTTPServer) serve(ctx *Context) {
	panic("implement me")
}

func (hs *HTTPServer) addRoute(method string, path string, handler HandleFunc) {
	panic("implement me")
}
