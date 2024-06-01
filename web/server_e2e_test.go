package web

import (
	"fmt"
	"testing"
)

type Person struct {
	Name    string
	Country string
}

func TestServer(t *testing.T) {
	httpServer := NewHttpServer()

	var p Person
	httpServer.Post("/login", func(ctx *Context) {
		//将客户端传入的json字符串转换为Person对象
		err := ctx.BindJSON(&p)
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Println(p)
	})
	err := httpServer.Start(":8080")
	if err != nil {
		t.Error(err)
	}
}
