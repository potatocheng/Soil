package concurrency

import (
	"context"
	"errors"
	"fmt"
	"log"
	"testing"
	"time"
)

func readContextValue(ctx context.Context) error {
	val, ok := ctx.Value("parent_key").(string)
	if !ok {
		return errors.New("key is not founds")
	}
	log.Println(val)
	return nil
}

func TestContext_WithValue(t *testing.T) {
	ctxBegin := context.Background()
	ctxParent := context.WithValue(ctxBegin, "parent_key", "hello parent context")
	ctxChild := context.WithValue(ctxParent, "child_key", "hello child context")
	errCh := make(chan error)
	go func() {
		errCh <- readContextValue(ctxChild)
	}()
	err := <-errCh
	if err != nil {
		log.Println(err)
	} else {
		log.Println("Success")
	}
}

func TestContext_WithCancel(t *testing.T) {
	parentCtx, cancel := context.WithCancel(context.Background())
	//middleCtx := context.WithValue(parentCtx, "key", "hello middleCtx")
	childCtx, childCancel := context.WithCancel(parentCtx)

	go func() {
		select {
		case <-childCtx.Done():
			fmt.Println("Child context cancelled")
		case <-time.After(2 * time.Second):
			fmt.Println("Child context finished work")
		}
	}()

	time.Sleep(1 * time.Second)
	cancel()

	// 等待子上下文的goroutine打印输出
	time.Sleep(2 * time.Second)

	// 子上下文取消函数也要调用，以防止资源泄漏
	childCancel()
}

func TestContext_WithDeadline(t *testing.T) {
	ctxParent, cancelParent := context.WithDeadline(context.Background(), time.Now().Add(time.Second*10))
	defer cancelParent()
	ctxChild, cancelChild := context.WithDeadline(ctxParent, time.Now().Add(time.Second*5))
	defer cancelChild()
	deadline, _ := ctxChild.Deadline()
	fmt.Println(deadline)

	ctxParent.Done()
}

func TestContext_Done(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	time.Sleep(time.Second * 3)
	defer cancel()

	go func() {
		select {
		case <-ctx.Done():
			fmt.Println("Context cancelled")
		}
	}()
}
