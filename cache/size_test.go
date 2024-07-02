package cache

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_sizeOf(t *testing.T) {
	type User struct {
		name int8  // 1
		id   int64 // 8
	}
	testCases := []struct {
		name       string
		val        any
		wantResult int
		wantErr    error
	}{
		{
			name:       "string",
			val:        string("hello world!!!!"),
			wantResult: 16 + 15,
		},
		{
			name:       "array",
			val:        [3]int{1, 2, 3},
			wantResult: 24,
		},
		{
			name:       "slice",
			val:        []int{1, 2, 3},
			wantResult: 24 + 24,
		},
		{
			name:       "pointer",
			val:        new(int),
			wantResult: 8 + 8,
		},
		{
			name:       "map",
			val:        map[string]int{"a": 1},
			wantResult: 8 + 17*1 + 8,
		},
		{
			name:       "map",
			val:        map[string]int{"a": 1},
			wantResult: 8 + 17*1 + 8*1,
		},
		{
			name:       "struct",
			val:        User{},
			wantResult: 16,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := Of(tc.val)
			assert.Equal(t, err, tc.wantErr)
			if err != nil {
				return
			}
			assert.Equal(t, tc.wantResult, res)
		})
	}
}
