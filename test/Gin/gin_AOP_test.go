package GinAOP

import (
	"github.com/gin-gonic/gin"
	"log"
	"testing"
	"time"
)

func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		latency := time.Since(start)
		log.Printf("Request: %s %s | Status: %d | Latency: %v", c.Request.Method, c.Request.URL.Path, c.Writer.Status(), latency)
	}
}

func Auth() gin.HandlerFunc {
	return func(context *gin.Context) {
		token := context.GetHeader("Authorization")

		if token != "valid-token" {
			context.JSON(401, gin.H{"error": "Unauthorized"})
			context.Abort()
			return
		}

		context.Next()
	}
}

func Test_GINAOP(t *testing.T) {
	r := gin.Default()

	r.Use(Logger(), Auth())

	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "pong",
		})
	})

	err := r.Run(":8080")
	if err != nil {
		panic("web: http server start failed")
	}
}
