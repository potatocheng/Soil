package DataStruct

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"reflect"
	"testing"
	"unsafe"
)

func TestDsOptimization(t *testing.T) {

	str1 := "hello world"
	str2 := str1
	s1Ptr := uintptr(unsafe.Pointer(unsafe.StringData(str1)))
	s2Ptr := uintptr(unsafe.Pointer(unsafe.StringData(str2)))
	fmt.Printf("s1Ptr: %d\n", s1Ptr)
	fmt.Printf("s2Ptr: %d\n", s2Ptr)
	assert.Equal(t, s1Ptr, s2Ptr)

	// slice和map的赋值操作只是复制了引用，在赋值后没有发生扩容，他们还是共用一个底层
	slice1 := make([]int, 0, 10)
	for i := 0; i < 5; i++ {
		slice1 = append(slice1, i)
	}

	slice2 := slice1
	slice1Ptr := reflect.ValueOf(slice1).Pointer()
	slice2Ptr := reflect.ValueOf(slice2).Pointer()
	fmt.Printf("slice1Ptr: %d\n", slice1Ptr)
	fmt.Printf("slice2Ptr: %d\n", slice2Ptr)
	assert.Equal(t, slice1Ptr, slice2Ptr)

	slice2 = append(slice2, 6)

	fmt.Printf("slice1: %v\n", slice1)
	fmt.Printf("slice2: %v\n", slice2)

	slice1Ptr = reflect.ValueOf(slice1).Pointer()
	slice2Ptr = reflect.ValueOf(slice2).Pointer()
	fmt.Printf("slice1Ptr: %d\n", slice1Ptr)
	fmt.Printf("slice2Ptr: %d\n", slice2Ptr)
	assert.NotEqual(t, slice1Ptr, slice2Ptr)

	slice3 := []int{1, 2, 3}
	slice4 := slice3
	fmt.Printf("slice3:%p\n", unsafe.Pointer(&slice3))
	fmt.Printf("slice4:%p\n", unsafe.Pointer(&slice4))

	//map 是引用类型，赋值操作只是复制了引用，而不是复制整个 map
	map1 := map[int]int{1: 1, 2: 1, 3: 1}
	map2 := map1
	map1Ptr := reflect.ValueOf(map1).Pointer()
	map2Ptr := reflect.ValueOf(map2).Pointer()
	fmt.Printf("map1Ptr: %d\n", map1Ptr)
	fmt.Printf("map2Ptr: %d\n", map2Ptr)
	assert.Equal(t, map1Ptr, map2Ptr)

	map2[4] = 1
	for k, v := range map1 {
		fmt.Printf("k: %d, v: %d ", k, v)
	}
	fmt.Println()
	for k, v := range map2 {
		fmt.Printf("k: %d, v: %d ", k, v)
	}
	fmt.Println()
	map1Ptr = reflect.ValueOf(map1).Pointer()
	map2Ptr = reflect.ValueOf(map2).Pointer()
	fmt.Printf("map1Ptr: %d\n", map1Ptr)
	fmt.Printf("map2Ptr: %d\n", map2Ptr)
	assert.Equal(t, map1Ptr, map2Ptr)
}
