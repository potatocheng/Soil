package DataStruct

import (
	"fmt"
	"reflect"
	"testing"
	"unsafe"
)

// 字符串是字符组成的数组, Go语言中的字符串是一个只读的字节数组，如果要修改需要转换成[]byte类型
func TestString(t *testing.T) {
	s1 := "hello"

	// 获取了指向s1的指针的地址的整数表示。
	str := "Hello"
	strPtr := (*reflect.StringHeader)(unsafe.Pointer(&str)).Data

	// 复制字符串
	s2 := s1

	// 获取s2的指针
	s2Ptr := *(*uintptr)(unsafe.Pointer(&s2))

	// 检查s1和s2是否指向相同的内存地址
	fmt.Printf("s1的地址: %d\ns2的地址: %d\n", s1Ptr, s2Ptr)
	fmt.Println("s1 and s2 point to the same address:", s1Ptr == s2Ptr)
}
