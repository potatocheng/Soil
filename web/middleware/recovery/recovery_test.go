package recovery

import (
	"Soil/web"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRecovery(t *testing.T) {
	httpServer := web.NewHttpServer()

	recoveryBuilder := Create()
	recoveryMiddleware := recoveryBuilder.Build()

	httpServer.Use(recoveryMiddleware)
	httpServer.Get("/panic", func(ctx *web.Context) {
		panic("test panic")
	})

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	w := httptest.NewRecorder()

	httpServer.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "Internal Server Error")
}

func TestRecovery_NoPanic(t *testing.T) {
	httpServer := web.NewHttpServer()

	recoveryBuilder := Create()
	recoveryMiddleware := recoveryBuilder.Build()

	httpServer.Use(recoveryMiddleware)
	httpServer.Get("/hello", func(ctx *web.Context) {
		ctx.RespJson(http.StatusOK, map[string]string{"message": "hello"})
	})

	req := httptest.NewRequest(http.MethodGet, "/hello", nil)
	w := httptest.NewRecorder()

	httpServer.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}