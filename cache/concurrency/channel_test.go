package concurrency

import (
	"fmt"
	"testing"
)

func TestChannel(t *testing.T) {
	// 创建一个无缓冲channel
	c := make(chan int)
	// 写一个值
	go func() { c <- 1 }()
	i := <-c
	fmt.Printf("i = %d\n", i)
}

func returnChannel() <-chan int {
	c := make(chan int)
	c <- 1
	return c
}

func TestChannel_AsFuncReturnValue(t *testing.T) {
	go func() {
		select {
		case c := <-returnChannel():
			fmt.Println("ok---", c)
		}
	}()
}
