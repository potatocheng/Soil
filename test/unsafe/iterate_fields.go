package unsafe

import (
	"fmt"
	"reflect"
)

func IterateFields(entity any) {
	typ := reflect.TypeOf(entity)
	for i := 0; i < typ.NumField(); i++ {
		fd := typ.Field(i)
		fmt.Printf("%s %d\n", fd.Name, fd.Offset)
	}
}
