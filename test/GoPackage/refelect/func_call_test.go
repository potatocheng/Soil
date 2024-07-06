package refelect

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"reflect"
	"testing"
)

type Usr struct {
}

func (u *Usr) A(a int) {
	fmt.Println(a)
}

func TestFuncCall(t *testing.T) {
	refTyp := reflect.TypeOf(&Usr{})
	refVal := reflect.ValueOf(&Usr{})
	methodTyp, ok := refTyp.MethodByName("A")
	require.True(t, ok)
	fmt.Println("methodTyp numIn = ", methodTyp.Type.NumIn())
	methodVal := refVal.MethodByName("A")
	fmt.Println("methodVal numIn = ", methodVal.Type().NumIn())
}
