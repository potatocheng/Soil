package refelect

import (
	"errors"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestIterateFields(t *testing.T) {
	type User struct {
		Name string
		Age  int
	}

	type Order struct {
		Id   int
		name string
	}

	testCases := []struct {
		name   string
		entity any

		wantResult map[string]any
		wantErr    error
	}{
		{
			name:    "nil",
			entity:  nil,
			wantErr: errors.New("entity is nil"),
		},
		{
			name: "all exported filed struct",
			entity: User{
				Name: "John", Age: 18,
			},
			wantResult: map[string]any{
				"Name": "John",
				"Age":  18,
			},
		},
		{
			name: "unexported filed struct",
			entity: Order{
				Id: 1, name: "iPhone",
			},
			wantResult: map[string]any{
				"Id": 1, "name": "",
			},
		},
		{
			name: "pointer",
			entity: &User{
				Name: "John", Age: 18,
			},
			wantResult: map[string]any{
				"Name": "John", "Age": 18,
			},
		},
		{
			name: "multiple pointer",
			entity: func() **User {
				res := &User{
					Name: "John", Age: 18,
				}

				return &res
			}(),
			wantResult: map[string]any{
				"Name": "John", "Age": 18,
			},
		},
		{
			name:    "user nil",
			entity:  (*User)(nil),
			wantErr: errors.New("entity is zero"),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			res, err := IterateFields(testCase.entity)
			assert.Equal(t, testCase.wantErr, err)
			if err != nil {
				return
			}
			for key, value := range res {
				assert.Equal(t, testCase.wantResult[key], value)
			}
		})
	}
}

func TestSetField(t *testing.T) {
	type User struct {
		Name string
		age  int
	}

	testCases := []struct {
		name   string
		entity any
		field  string
		value  any

		wantResult any
		wantErr    error
	}{
		{
			name:   "modify struct field failed(reflect: reflect.Value.Set using unaddressable value)",
			entity: User{Name: "yi", age: 18},
			field:  "Name",
			value:  "cheng",

			wantErr:    errors.New("field can't set"),
			wantResult: User{Name: "cheng", age: 18},
		},
		{
			name: "modify pointer field success",

			entity: &User{Name: "yi", age: 18},
			field:  "Name",
			value:  "cheng",

			//wantErr:    errors.New("field can't set"),
			wantResult: &User{Name: "cheng", age: 18},
		},
		{
			name: "modify unexported field",

			entity:  &User{Name: "yi", age: 18},
			field:   "age",
			value:   -18,
			wantErr: errors.New("field can't set"),
			//wantResult: &User{Name: "yi", age: -18},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			err := SetField(testCase.entity, testCase.field, testCase.value)
			assert.Equal(t, testCase.wantErr, err)
			if err != nil {
				return
			}

			assert.Equal(t, testCase.wantResult, testCase.entity)
		})
	}
}
