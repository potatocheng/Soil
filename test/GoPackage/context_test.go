package GoPackage

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// context用来在goroutine中传递上下文信息，在不同goroutine间同步请求特定数据、取消信号以及处理请求的截止日期
// context中包含上下文类型，可以使用background/TODO创建上下文
func handle(ctx context.Context, duration time.Duration) {
	select {
	case <-ctx.Done():
		fmt.Println("handle", ctx.Err())
	case <-time.After(duration):
		fmt.Println("process request with", duration)
	}
}

func Test_Goroutine_Single_Sync(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	go handle(ctx, 500*time.Millisecond)
	select {
	case <-ctx.Done():
		fmt.Println("main", ctx.Err())
	}
}
