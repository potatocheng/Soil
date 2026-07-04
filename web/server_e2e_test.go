package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

type Person struct {
	Name    string `json:"name"`
	Country string `json:"country"`
}

func TestServer_Get(t *testing.T) {
	httpServer := NewHttpServer()

	httpServer.Get("/hello", func(ctx *Context) {
		ctx.RespJson(http.StatusOK, map[string]string{"message": "hello world"})
	})

	req := httptest.NewRequest(http.MethodGet, "/hello", nil)
	w := httptest.NewRecorder()

	httpServer.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json; charset=utf-8", w.Header().Get("Content-Type"))
}

func TestServer_Post_BindJSON(t *testing.T) {
	httpServer := NewHttpServer()

	httpServer.Post("/login", func(ctx *Context) {
		var p Person
		err := ctx.BindJSON(&p)
		if err != nil {
			ctx.RespJson(http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		ctx.RespJson(http.StatusOK, p)
	})

	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	w := httptest.NewRecorder()

	httpServer.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestServer_PathParam(t *testing.T) {
	httpServer := NewHttpServer()

	httpServer.Get("/user/:id", func(ctx *Context) {
		id := ctx.PathValue("id")
		ctx.RespJson(http.StatusOK, map[string]string{"id": id.val})
	})

	req := httptest.NewRequest(http.MethodGet, "/user/123", nil)
	w := httptest.NewRecorder()

	httpServer.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestServer_NotFound(t *testing.T) {
	httpServer := NewHttpServer()

	httpServer.Get("/hello", func(ctx *Context) {
		ctx.RespJson(http.StatusOK, map[string]string{"message": "hello"})
	})

	req := httptest.NewRequest(http.MethodGet, "/notfound", nil)
	w := httptest.NewRecorder()

	httpServer.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestServer_Put_Delete_Patch(t *testing.T) {
	httpServer := NewHttpServer()

	httpServer.Put("/user/:id", func(ctx *Context) {
		ctx.RespJson(http.StatusOK, map[string]string{"method": "PUT", "id": ctx.PathValue("id").val})
	})

	httpServer.Delete("/user/:id", func(ctx *Context) {
		ctx.RespJson(http.StatusOK, map[string]string{"method": "DELETE", "id": ctx.PathValue("id").val})
	})

	httpServer.Patch("/user/:id", func(ctx *Context) {
		ctx.RespJson(http.StatusOK, map[string]string{"method": "PATCH", "id": ctx.PathValue("id").val})
	})

	testCases := []struct {
		name   string
		method string
		path   string
	}{
		{"PUT", http.MethodPut, "/user/123"},
		{"DELETE", http.MethodDelete, "/user/123"},
		{"PATCH", http.MethodPatch, "/user/123"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			w := httptest.NewRecorder()
			httpServer.ServeHTTP(w, req)
			assert.Equal(t, http.StatusOK, w.Code)
		})
	}
}