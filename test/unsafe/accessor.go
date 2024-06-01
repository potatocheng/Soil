package unsafe

import (
	"errors"
	"reflect"
	"unsafe"
)

type FieldMate struct {
	typ    reflect.Type
	offset uintptr
}

type Accessor struct {
	initAddr unsafe.Pointer
	fields   map[string]FieldMate
}

func newAccessor(entity any) *Accessor {
	typ := reflect.TypeOf(entity).Elem()
	val := reflect.ValueOf(entity)
	fields := make(map[string]FieldMate, typ.NumField())

	for i := 0; i < typ.NumField(); i++ {
		fd := typ.Field(i)
		fields[fd.Name] = FieldMate{
			typ:    fd.Type,
			offset: fd.Offset,
		}
	}

	return &Accessor{
		initAddr: val.UnsafePointer(),
		fields:   fields,
	}
}

func (a *Accessor) GetField(fieldName string) (any, error) {
	fieldMeta, ok := a.fields[fieldName]
	if !ok {
		return nil, errors.New("非法字段")
	}
	fieldAddr := unsafe.Pointer(uintptr(a.initAddr) + fieldMeta.offset)

	//NewAt 返回一个Value,表示指向指定类型值的指针,使用p作为该指针,所以要获得原来的值需要调用Elem()
	return reflect.NewAt(fieldMeta.typ, fieldAddr).Elem().Interface(), nil
}

func (a *Accessor) SetField(fieldName string, newVal any) error {
	fieldMeta, ok := a.fields[fieldName]
	if !ok {
		return errors.New("非法字段")
	}

	fieldAddress := unsafe.Pointer(uintptr(a.initAddr) + fieldMeta.offset)
	reflect.NewAt(fieldMeta.typ, fieldAddress).Elem().Set(reflect.ValueOf(newVal))

	return nil
}
