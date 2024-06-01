package refelect

import (
	"errors"
	"reflect"
)

func IterateFields(entity any) (map[string]any, error) {
	// //使用reflect.TypeOf和reflect.ValueOf能够获取Go语言中变量对应的反射对象。一旦获取反射对象，就得到与当前类型相关的数据和操作
	// //反射使用场景一:类型检查与动态调用：反射可以用于在运行时检查变量的类型，并根据类型执行不同的操作。这在编写需要处理多种类型的代码时特别有用，例如通用库或框架。
	// var x float32 = 3.4
	// fmt.Println("type: ", reflect.TypeOf(x))   // reflect.TypeOf函数获取任意变量的类型, 返回reflect.Type接口
	// fmt.Println("value: ", reflect.ValueOf(x)) // reflect.ValueOf返回reflect.Value结构体
	//
	// //反射对象还原成接口类型的变量， reflect.Value.Interface
	// v := reflect.ValueOf(1)
	// var _ int = v.Interface().(int)

	if entity == nil {
		return nil, errors.New("entity is nil")
	}

	refTyp := reflect.TypeOf(entity)
	refVal := reflect.ValueOf(entity)

	if refVal.IsZero() {
		return nil, errors.New("entity is zero")
	}

	for refTyp.Kind() == reflect.Ptr {
		refTyp = refTyp.Elem()
		refVal = refVal.Elem()
	}

	if refTyp.Kind() != reflect.Struct {
		//当refTyp是nil时调用kind会panic
		return nil, errors.New("entity is not a struct")
	}

	var res map[string]any = make(map[string]any, refTyp.NumField())
	for i := 0; i < refTyp.NumField(); i++ {
		fieldType := refTyp.Field(i)
		fieldVal := refVal.Field(i)
		if fieldType.IsExported() {
			// Do not allow access to unexported values via Interface
			res[fieldType.Name] = fieldVal.Interface()
		} else {
			res[fieldType.Name] = reflect.Zero(fieldType.Type).Interface()
		}
	}

	return res, nil
}

func SetField(entity any, field string, value any) error {
	refVal := reflect.ValueOf(entity)

	for refVal.Type().Kind() == reflect.Ptr {
		refVal = refVal.Elem()
	}

	fieldVal := refVal.FieldByName(field)
	if !fieldVal.CanSet() {
		return errors.New("field can't set")
	}

	fieldVal.Set(reflect.ValueOf(value))

	return nil
}
