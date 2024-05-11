package web

import "net/http"

type Context struct {
	Resp http.ResponseWriter
	Req  *http.Request
}
