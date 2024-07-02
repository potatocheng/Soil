package main

import (
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

func TestGen(t *testing.T) {
	f, err := os.Create("./testdata/user.gen.go")
	require.NoError(t, err)
	err = gen(f, "./testdata/user.go")
	require.NoError(t, err)
}
