package GoPackage

import (
	"fmt"
	"reflect"
	"testing"
)

func Add(a, b int) int { return a + b }

func TestFiled(t *testing.T) {
	type TestStruct struct {
		FirstName string
		Age       int
		LastName  string
	}

	var testStruct TestStruct
	structType := reflect.TypeOf(testStruct)
	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		fmt.Println("field_name: ", field.Name, ", field_type: ", field.Type)
		val := reflect.New(field.Type)
		fmt.Printf("type: %T, value: %v", val.Interface(), val.Interface())
	}
}

func TestFuncCall(t *testing.T) {
	val := reflect.ValueOf(Add)
	if val.Kind() != reflect.Func {
		return
	}
	typ := val.Type()
	// NumIn returns a function type's input parameter count.
	argv := make([]reflect.Value, typ.NumIn())
	for i := range argv {
		if typ.In(i).Kind() != reflect.Int {
			return
		}
		argv[i] = reflect.ValueOf(i)
	}

	result := val.Call(argv)
	if len(result) != 1 || result[0].Kind() != reflect.Int {
		return
	}
	fmt.Println(result[0].Int())
}
