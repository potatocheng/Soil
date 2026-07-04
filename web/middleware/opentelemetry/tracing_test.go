package opentelemetry

import (
	"Soil/web"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTracingMiddleware(t *testing.T) {
	httpServer := web.NewHttpServer()

	tracerBuilder := &MiddlewareBuilder{}
	tracerMiddleware := tracerBuilder.Build()

	httpServer.Use(tracerMiddleware)
	httpServer.Get("/hello", func(ctx *web.Context) {
		ctx.RespJson(http.StatusOK, map[string]string{"message": "hello"})
	})

	req := httptest.NewRequest(http.MethodGet, "/hello", nil)
	w := httptest.NewRecorder()

	httpServer.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestTracingMiddleware_Error(t *testing.T) {
	httpServer := web.NewHttpServer()

	tracerBuilder := &MiddlewareBuilder{}
	tracerMiddleware := tracerBuilder.Build()

	httpServer.Use(tracerMiddleware)
	httpServer.Get("/error", func(ctx *web.Context) {
		ctx.RespJson(http.StatusInternalServerError, map[string]string{"error": "something went wrong"})
	})

	req := httptest.NewRequest(http.MethodGet, "/error", nil)
	w := httptest.NewRecorder()

	httpServer.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}