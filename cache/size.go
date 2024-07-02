package cache

import (
	"errors"
	"fmt"
	"reflect"
	"sync"
	"unsafe"
)

type cache struct {
	cache   map[uintptr]bool
	rwMutex sync.RWMutex
}

func newCache() *cache {
	return &cache{make(map[uintptr]bool), sync.RWMutex{}}
}

func (c *cache) keyIsExist(key uintptr) bool {
	c.rwMutex.RLock()
	defer c.rwMutex.RUnlock()
	f, ok := c.cache[key]
	// 没有std::set只能这样
	return f && ok
}

func (c *cache) set(key uintptr) {
	c.rwMutex.Lock()
	defer c.rwMutex.Unlock()
	c.cache[key] = true
}

func Of(val any) (uint32, error) {
	cache := newCache()
	size, err := sizeOf(reflect.ValueOf(val), cache)
	if err != nil {
		return 0, err
	}

	return uint32(size), nil
}

// sizeOf cache参数防止多次计算相同内容，如
func sizeOf(val reflect.Value, cache *cache) (int, error) {
	switch val.Kind() {
	case reflect.Bool,
		reflect.Chan,
		reflect.Func,
		reflect.Uintptr,
		reflect.Float32, reflect.Float64,
		reflect.Complex64, reflect.Complex128,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int(val.Type().Size()), nil
	case reflect.String:
		s := val.String()
		strDataPtr := uintptr(unsafe.Pointer(unsafe.StringData(s)))
		if cache.keyIsExist(strDataPtr) {
			return int(val.Type().Size()), nil
		}
		cache.set(strDataPtr)
		return int(val.Type().Size()) + len(s), nil
	case reflect.Array:
		sum := 0
		for i := 0; i < val.Len(); i++ {
			s, err := sizeOf(val.Index(i), cache)
			if err != nil || s < 0 {
				return -1, err
			}
			sum += s
		}
		return sum + (val.Cap()-val.Len())*int(val.Type().Elem().Size()), nil
	case reflect.Slice:
		if cache.keyIsExist(val.Pointer()) {
			return 0, nil
		}
		cache.set(val.Pointer())
		sum := 0
		for i := 0; i < val.Len(); i++ {
			s, err := sizeOf(val.Index(i), cache)
			if err != nil || s < 0 {
				return -1, err
			}
			sum += s
		}

		return sum + int(val.Type().Size()) + (val.Cap()-val.Len())*int(val.Type().Elem().Size()), nil
	case reflect.Ptr:
		if cache.keyIsExist(val.Pointer()) {
			return int(val.Type().Size()), nil
		}
		cache.set(val.Pointer())
		if val.IsNil() {
			return int(reflect.New(val.Type()).Type().Size()), nil
		}
		s, err := sizeOf(val.Elem(), cache)
		if err != nil || s < 0 {
			return -1, err
		}
		return s + int(val.Type().Size()), nil
	case reflect.Interface:
		s, err := sizeOf(val.Elem(), cache)
		if err != nil || s < 0 {
			return -1, err
		}
		return s + int(val.Type().Size()), nil
	case reflect.Map:
		if cache.keyIsExist(val.Pointer()) {
			return 0, nil
		}
		cache.set(val.Pointer())
		keys := val.MapKeys()
		sum := 0
		for _, key := range keys {
			v := val.MapIndex(key)
			//计算val的内存大小
			sv, err := sizeOf(v, cache)
			if err != nil || sv < 0 {
				return -1, err
			}
			sum += sv
			// 计算key的内存大小
			fmt.Println("kind of key is ", key.Kind().String())
			sk, err := sizeOf(key, cache)
			if err != nil || sk < 0 {
				return -1, err
			}
			sum += sk
		}
		return sum + int(val.Type().Size()), nil
	case reflect.Struct:
		numField := val.Type().NumField()
		sum := 0
		for i := 0; i < numField; i++ {
			field := val.Field(i)
			s, err := sizeOf(field, cache)
			if err != nil || s < 0 {
				return -1, err
			}
			sum += s
		}

		// 考虑struct里为了内存对齐加入的padding
		padding := int(val.Type().Size())
		for i := 0; i < numField; i++ {
			padding -= int(val.Field(i).Type().Size())
		}

		return sum + padding, nil

	default:
		return -1, errors.New("unsupported type: " + val.Kind().String())
	}
}
