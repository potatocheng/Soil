package refelect

import "reflect"

func IterateFunc(entity any) (map[string]FuncInfo, error) {
	typ := reflect.TypeOf(entity)
	numMethod := typ.NumMethod() // 获得传入entity(struct)实现的方法数量
	res := make(map[string]FuncInfo, numMethod)
	for i := 0; i < numMethod; i++ {
		method := typ.Method(i) //获取方法，方法按照字母顺序排序
		fn := method.Func       // func with receiver as first argument

		numIn := method.Type.NumIn() //获得函数入参数量
		inputTypes := make([]reflect.Type, 0, numIn)
		inputValues := make([]reflect.Value, 0, numIn)

		// func with receiver as first argument, 所以第一个参数需要传入receiver的值
		inputValues = append(inputValues, reflect.ValueOf(entity))
		inputTypes = append(inputTypes, reflect.TypeOf(entity))

		for j := 1; j < numIn; j++ {
			fnInType := method.Type.In(j)
			inputTypes = append(inputTypes, fnInType)
			inputValues = append(inputValues, reflect.Zero(fnInType))
		}

		numOut := method.Type.NumOut()
		outputTypes := make([]reflect.Type, 0, numOut)
		for j := 0; j < numOut; j++ {
			outputTypes = append(outputTypes, method.Type.Out(j))
		}

		resValues := fn.Call(inputValues)
		result := make([]any, 0, len(resValues))
		for _, v := range resValues {
			result = append(result, v.Interface())
		}

		res[method.Name] = FuncInfo{
			FuncName:    method.Name,
			InputTypes:  inputTypes,
			OutputTypes: outputTypes,
			Result:      result,
		}
	}

	return res, nil
}

type FuncInfo struct {
	FuncName    string
	InputTypes  []reflect.Type //函数形参的类型
	OutputTypes []reflect.Type //返回值类型
	Result      []any          //返回值结果
}
