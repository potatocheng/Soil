package Model

import (
	"fmt"
	"testing"
)

func spawn() chan int {
	c := make(chan int)
	go func() {
		c <- 1
		i := <-c
		fmt.Println("spawn goroutine i:", i)
	}()
	return c
}

func TestModel(t *testing.T) {
	// 1.创建模式，函数创建的新goroutine与调用函数的goroutine之间通过一个channel建立联系
	// 拿到在spawn中创建的chan来操作spawn中的goroutine
	c := spawn()
	i := <-c
	c <- 2
	fmt.Println("main goroutine i :", i)

	// 2.退出模式
	// (1) 分离模式：创建的goroutine不需要惯性它的退出
	// 用途1：完成一次性任务
	// 用途2：常驻后台执行一些特定任务
	// (2) join模式
}
