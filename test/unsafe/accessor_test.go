package unsafe

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAccessor(t *testing.T) {
	type User struct {
		Name string
		Age  uint16
	}

	u := &User{Name: "yang", Age: 18}
	accessor := newAccessor(u)
	fmt.Println(accessor)

	val, err := accessor.GetField("Name")
	require.NoError(t, err)
	assert.Equal(t, val, "yang")

	err = accessor.SetField("Age", uint16(200))
	require.NoError(t, err)
	assert.Equal(t, u.Age, uint16(200))
}
