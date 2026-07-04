package errhdl

import (
	"Soil/web"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrHdl(t *testing.T) {
	httpServer := web.NewHttpServer()

	errBuilder := NewMiddlewareBuilder()
	errBuilder.RegisterError(http.StatusNotFound, []byte("Custom 404 Page"))
	errBuilder.RegisterError(http.StatusInternalServerError, []byte("Custom 500 Page"))
	errMiddleware := errBuilder.Build()

	httpServer.Use(errMiddleware)
	httpServer.Get("/hello", func(ctx *web.Context) {
		ctx.RespStatusCode = http.StatusNotFound
	})

	req := httptest.NewRequest(http.MethodGet, "/hello", nil)
	w := httptest.NewRecorder()

	httpServer.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "Custom 404 Page")
}

func TestErrHdl_NoRegisteredError(t *testing.T) {
	httpServer := web.NewHttpServer()

	errBuilder := NewMiddlewareBuilder()
	errMiddleware := errBuilder.Build()

	httpServer.Use(errMiddleware)
	httpServer.Get("/hello", func(ctx *web.Context) {
		ctx.RespStatusCode = http.StatusBadRequest
	})

	req := httptest.NewRequest(http.MethodGet, "/hello", nil)
	w := httptest.NewRecorder()

	httpServer.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}