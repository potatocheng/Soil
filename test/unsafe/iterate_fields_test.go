package unsafe

import (
	"testing"
)

func TestIterateFields(t *testing.T) {
	testCases := []struct {
		name   string
		entity any
	}{
		{
			name:   "user",
			entity: user{},
		},
		{
			name:   "userV1",
			entity: userV1{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			IterateFields(tc.entity)
		})
	}
}

type user struct {
	Name    string
	Age     int32
	Alias   []string
	address string
}

type userV1 struct {
	Name    string
	Age     int32
	AgeV1   int32
	Alias   []string
	address string
}
