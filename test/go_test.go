package test

import (
	"fmt"
	"strings"
	"testing"
)

func TestGo(t *testing.T) {
	s := "/hello/world/this//is a /good year"
	//去掉首尾的cutset
	var x string = strings.Trim(s, "/")
	fmt.Println(x)

	seg := strings.Split(s, "/")

	for k, v := range seg {
		fmt.Println(k, v)
	}

	//map[string]int64
}
