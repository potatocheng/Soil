package valuer

import (
	"Soil/orm/internal/model"
	"database/sql/driver"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_reflectValue_SetColumns(t *testing.T) {
	testCases := []struct {
		name    string
		cs      map[string][]byte
		entity  *SimpleStruct
		wantVal *SimpleStruct
	}{
		{
			name: "normal value",
			cs: map[string][]byte{
				"id":               []byte("1"),
				"bool":             []byte("true"),
				"bool_ptr":         []byte("false"),
				"int":              []byte("12"),
				"int_ptr":          []byte("13"),
				"int8":             []byte("8"),
				"int8_ptr":         []byte("-8"),
				"int16":            []byte("16"),
				"int16_ptr":        []byte("-16"),
				"int32":            []byte("32"),
				"int32_ptr":        []byte("-32"),
				"int64":            []byte("64"),
				"int64_ptr":        []byte("-64"),
				"uint":             []byte("14"),
				"uint_ptr":         []byte("15"),
				"uint8":            []byte("8"),
				"uint8_ptr":        []byte("18"),
				"uint16":           []byte("16"),
				"uint16_ptr":       []byte("116"),
				"uint32":           []byte("32"),
				"uint32_ptr":       []byte("132"),
				"uint64":           []byte("64"),
				"uint64_ptr":       []byte("164"),
				"float32":          []byte("3.2"),
				"float32_ptr":      []byte("-3.2"),
				"float64":          []byte("6.4"),
				"float64_ptr":      []byte("-6.4"),
				"byte":             []byte("8"),
				"byte_ptr":         []byte("18"),
				"byte_array":       []byte("hello"),
				"string":           []byte("world"),
				"null_string_ptr":  []byte("null string"),
				"null_int16_ptr":   []byte("16"),
				"null_int32_ptr":   []byte("32"),
				"null_int64_ptr":   []byte("64"),
				"null_bool_ptr":    []byte("true"),
				"null_float64_ptr": []byte("6.4"),
				"json_column":      []byte(`{"name": "Tom", "age" : 18}`),
			},
			entity:  &SimpleStruct{},
			wantVal: NewSimpleStruct(1),
		},
	}

	r := model.NewRegistry()
	meta, err := r.Get(&SimpleStruct{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			refelctValuer := NewReflectValue(tc.entity, meta)
			//使用 sqlmock 构建sql.Rows
			cols := make([]string, 0, len(tc.cs))
			colVals := make([]driver.Value, 0, len(tc.cs))
			for k, v := range tc.cs {
				cols = append(cols, k)
				colVals = append(colVals, v)
			}

			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			mock.ExpectQuery("SELECT *").WillReturnRows(
				mock.NewRows(cols).AddRow(colVals...))
			rows, err := db.Query("SELECT *")
			if err != nil {
				t.Fatal(err)
			}
			if !rows.Next() {
				t.Fatal("没有拿到列")
			}
			err = refelctValuer.SetColumns(rows)
			if err != nil {
				assert.Equal(t, err, tc.wantVal)
			}
			assert.Equal(t, tc.wantVal, tc.entity)
		})
	}
}
