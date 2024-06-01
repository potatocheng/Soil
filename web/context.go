package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
)

type Context struct {
	Resp       http.ResponseWriter
	Req        *http.Request
	PathParams map[string]string

	RespStatusCode int
	RespData       []byte

	MatchedRoute string

	cacheQueryValues url.Values
}

type StringValue struct {
	val string
	err error
}

// BindJSON 将Body里的JSON数据转换为struct, BindJSON(&obj)
func (c *Context) BindJSON(val any) error {
	if c.Req.Body == nil {
		return errors.New("web: body is nil")
	}

	decoder := json.NewDecoder(c.Req.Body)
	return decoder.Decode(val)
}

// FormValue 处理表单数据
func (c *Context) FormValue(key string) StringValue {
	//只会parse一次
	err := c.Req.ParseForm()
	if err != nil {
		return StringValue{err: err}
	}

	return StringValue{val: c.Req.FormValue(key)}
}

// QueryValue 处理查询参数
func (c *Context) QueryValue(key string) StringValue {
	// c.Req.URL.Query()不会创建缓存，所以可以先将它缓存起来
	if c.cacheQueryValues == nil {
		c.cacheQueryValues = c.Req.URL.Query()
	}

	vals, ok := c.cacheQueryValues[key]
	if !ok {
		return StringValue{err: errors.New("web: key not found")}
	}

	return StringValue{val: vals[0]}
}

// PathValue 处理路径参数
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

func (c *Context) RespJson(code int, val any) error {
	bs, err := json.Marshal(val)
	if err != nil {
		return err
	}

	c.Resp.WriteHeader(code)
	_, err = c.Resp.Write(bs)
	return err
}
