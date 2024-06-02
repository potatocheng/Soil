package refelect

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

// 这里接收指针
func ModifyValueSuccess(entity any, field string, val any) error {
	refVal := reflect.ValueOf(entity)

	if refVal.Type().Kind() != reflect.Ptr {
		return errors.New("entity is not a pointer")
	}

	refVal.Elem().FieldByName(field).Set(reflect.ValueOf(val))

	return nil
}

// 这里接收值
func ModifyValueFail(entity any, field string, val any) error {
	refVal := reflect.ValueOf(entity)

	fd := refVal.FieldByName(field)
	if !fd.CanSet() {
		return fmt.Errorf("field %s is not settable", field)
	}

	fd.Set(reflect.ValueOf(val))

	return nil
}

type User struct {
	Name string
}

func TestReflect_Pointer(t *testing.T) {
	u := User{Name: "Tom"}
	err := ModifyValueSuccess(&u, "Name", "Jerry")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(u)

	err = ModifyValueFail(&u, "Name", "Fari")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(u)
}
