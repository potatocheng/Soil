package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
)

type Context struct {
	Resp       http.ResponseWriter
	Req        *http.Request
	PathParams map[string]string

	RespStatusCode int
	RespData       []byte
	RespHeaders    http.Header

	MatchedRoute string
	RequestID    string

	cacheQueryValues url.Values
	done             bool
}

type StringValue struct {
	val string
	err error
}

func (sv StringValue) Value() string {
	return sv.val
}

func (sv StringValue) Error() error {
	return sv.err
}

func (c *Context) BindJSON(val any) error {
	contentType := strings.ToLower(strings.TrimSpace(c.Req.Header.Get("Content-Type")))
	if idx := strings.Index(contentType, ";"); idx >= 0 {
		contentType = contentType[:idx]
	}
	contentType = strings.TrimSpace(contentType)
	if contentType != "application/json" {
		return errors.New("web: Content-Type is not application/json")
	}

	if c.Req.Body == nil {
		return errors.New("web: body is nil")
	}

	decoder := json.NewDecoder(c.Req.Body)
	return decoder.Decode(val)
}

func (c *Context) FormValue(key string) StringValue {
	err := c.Req.ParseForm()
	if err != nil {
		return StringValue{err: err}
	}

	return StringValue{val: c.Req.FormValue(key)}
}

func (c *Context) QueryValue(key string) StringValue {
	if c.cacheQueryValues == nil {
		c.cacheQueryValues = c.Req.URL.Query()
	}

	vals, ok := c.cacheQueryValues[key]
	if !ok {
		return StringValue{err: errors.New("web: key not found")}
	}

	return StringValue{val: vals[0]}
}

func (c *Context) PathValue(key string) StringValue {
	val, ok := c.PathParams[key]
	if !ok {
		return StringValue{err: errors.New("web: key not found")}
	}
	return StringValue{val: val}
}

func (c *Context) SetCookie(cookie *http.Cookie) {
	http.SetCookie(c.Resp, cookie)
}

func (c *Context) SetHeader(key, value string) {
	if c.RespHeaders == nil {
		c.RespHeaders = make(http.Header)
	}
	c.RespHeaders.Set(key, value)
}

func (c *Context) GetHeader(key string) string {
	return c.Req.Header.Get(key)
}

func (c *Context) RespJson(code int, val any) error {
	bs, err := json.Marshal(val)
	if err != nil {
		return err
	}

	c.SetHeader("Content-Type", "application/json; charset=utf-8")
	c.RespStatusCode = code
	c.RespData = bs
	return nil
}

func (c *Context) RespString(code int, format string, values ...any) error {
	c.SetHeader("Content-Type", "text/plain; charset=utf-8")
	c.RespStatusCode = code
	c.RespData = []byte(fmt.Sprintf(format, values...))
	return nil
}

func (c *Context) RespHTML(code int, html string) error {
	c.SetHeader("Content-Type", "text/html; charset=utf-8")
	c.RespStatusCode = code
	c.RespData = []byte(html)
	return nil
}

func (c *Context) FormFile(key string) (multipart.File, *multipart.FileHeader, error) {
	return c.Req.FormFile(key)
}

func (c *Context) GetRawData() ([]byte, error) {
	return io.ReadAll(c.Req.Body)
}

func (c *Context) Abort(code int, msg string) {
	c.done = true
	c.RespStatusCode = code
	c.RespData = []byte(msg)
}

func (c *Context) IsDone() bool {
	return c.done
}

func (c *Context) GetStatusCode() int {
	return c.RespStatusCode
}

func (c *Context) GetRequestID() string {
	return c.RequestID
}

// reset 清空 Context 所有字段，使其可被 sync.Pool 安全复用。
// 引用类型字段（map/slice）置为 nil，下次使用时由对应方法（如 SetHeader、
// QueryValue）重新 make，避免长期持有旧数据导致内存泄漏。
// 该方法仅由 HTTPServer.ServeHTTP 在请求处理前后调用，外部代码不应直接调用。
func (c *Context) reset() {
	c.Req = nil
	c.Resp = nil
	c.PathParams = nil
	c.RespStatusCode = 0
	c.RespData = nil
	c.RespHeaders = nil
	c.MatchedRoute = ""
	c.RequestID = ""
	c.cacheQueryValues = nil
	c.done = false
}
