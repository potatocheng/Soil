package accesslog

import (
	"Soil/web"
	"log"
	"testing"
)

func TestAccessLog(t *testing.T) {
	httpServer := web.NewHttpServer()

	logBuilder := Create()
	logMiddleware := logBuilder.Build()

	httpServer.Use(logMiddleware)
	httpServer.Get("/", func(ctx *web.Context) {
		ctx.Resp.Write([]byte("hello world"))
	})

	err := httpServer.Start(":8080")
	if err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
