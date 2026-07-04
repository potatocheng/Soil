package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRespString_WithFormat(t *testing.T) {
	ctx := &Context{
		Req:  httptest.NewRequest(http.MethodGet, "/", nil),
		Resp: httptest.NewRecorder(),
	}
	err := ctx.RespString(http.StatusOK, "hello %s, you are %d", "tom", 18)
	assert.NoError(t, err)
	assert.Equal(t, "hello tom, you are 18", string(ctx.RespData))
	assert.Equal(t, http.StatusOK, ctx.RespStatusCode)
	assert.Equal(t, "text/plain; charset=utf-8", ctx.RespHeaders.Get("Content-Type"))
}

func TestRespString_WithSingleArg(t *testing.T) {
	ctx := &Context{
		Req:  httptest.NewRequest(http.MethodGet, "/", nil),
		Resp: httptest.NewRecorder(),
	}
	err := ctx.RespString(http.StatusOK, "hello %s", "tom")
	assert.NoError(t, err)
	assert.Equal(t, "hello tom", string(ctx.RespData))
	assert.Equal(t, http.StatusOK, ctx.RespStatusCode)
	assert.Equal(t, "text/plain; charset=utf-8", ctx.RespHeaders.Get("Content-Type"))
}

func TestRespString_NoArgs(t *testing.T) {
	ctx := &Context{
		Req:  httptest.NewRequest(http.MethodGet, "/", nil),
		Resp: httptest.NewRecorder(),
	}
	err := ctx.RespString(http.StatusOK, "no args")
	assert.NoError(t, err)
	assert.Equal(t, "no args", string(ctx.RespData))
	assert.Equal(t, http.StatusOK, ctx.RespStatusCode)
	assert.Equal(t, "text/plain; charset=utf-8", ctx.RespHeaders.Get("Content-Type"))
}

func TestRespString_StatusCode(t *testing.T) {
	ctx := &Context{
		Req:  httptest.NewRequest(http.MethodGet, "/", nil),
		Resp: httptest.NewRecorder(),
	}
	err := ctx.RespString(http.StatusBadRequest, "bad request: %s", "missing field")
	assert.NoError(t, err)
	assert.Equal(t, "bad request: missing field", string(ctx.RespData))
	assert.Equal(t, http.StatusBadRequest, ctx.RespStatusCode)
	assert.Equal(t, "text/plain; charset=utf-8", ctx.RespHeaders.Get("Content-Type"))
}

type bindJSONUser struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

func TestBindJSON_TR16_1_ValidJSON(t *testing.T) {
	body := strings.NewReader(`{"name":"tom","age":18}`)
	req := httptest.NewRequest(http.MethodPost, "/", body)
	req.Header.Set("Content-Type", "application/json")
	ctx := &Context{Req: req}

	var u bindJSONUser
	err := ctx.BindJSON(&u)
	assert.NoError(t, err)
	assert.Equal(t, "tom", u.Name)
	assert.Equal(t, 18, u.Age)
}

func TestBindJSON_TR16_2_TextPlain(t *testing.T) {
	body := strings.NewReader(`{"name":"tom","age":18}`)
	req := httptest.NewRequest(http.MethodPost, "/", body)
	req.Header.Set("Content-Type", "text/plain")
	ctx := &Context{Req: req}

	var u bindJSONUser
	err := ctx.BindJSON(&u)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Content-Type")
}

func TestBindJSON_TR16_3_WithCharset(t *testing.T) {
	body := strings.NewReader(`{"name":"jerry","age":20}`)
	req := httptest.NewRequest(http.MethodPost, "/", body)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	ctx := &Context{Req: req}

	var u bindJSONUser
	err := ctx.BindJSON(&u)
	assert.NoError(t, err)
	assert.Equal(t, "jerry", u.Name)
	assert.Equal(t, 20, u.Age)
}

func TestBindJSON_TR16_4_NoContentType(t *testing.T) {
	body := strings.NewReader(`{"name":"tom","age":18}`)
	req := httptest.NewRequest(http.MethodPost, "/", body)
	// 故意不设置 Content-Type
	ctx := &Context{Req: req}

	var u bindJSONUser
	err := ctx.BindJSON(&u)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "Content-Type")
}
