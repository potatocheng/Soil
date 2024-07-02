package keyword

import "testing"

// go语言select让Goroutine同时等待多个Channel可读或者可写发生，
// 在多个文件或者Channel状态改变之前，Select会一直阻塞当前线程或者Goroutine
// 其实这个概念和linux里select系统调用(不太了解windows里有没有)概念上差不多
// 只是select系统调用监听文件描述符，

// 1.select能在Channel上进行非阻塞的收发操作
func TestSelect_NonBlock(t *testing.T) {
	//通常情况下select语句确实会阻塞当前Goroutine，
	//但是如果select控制结构中包含default语句
	ch := make(chan int, 1)

	select {
	case i := <-ch:
		println(i)
	default:
		println("default")
	}
	ch <- 1
}

// 2.select在遇到多个Channel同时响应时，会随机处理一种情况
