package valuer

import (
	"Soil/orm/internal/model"
	"database/sql/driver"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_unsafeValue_GetFieldValue(t *testing.T) {
	registry := model.NewRegistry()
	meta, err := registry.Get(&SimpleStruct{})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("零值字段可正常获取", func(t *testing.T) {
		// 全部字段都是零值的结构体
		entity := &SimpleStruct{}
		val := NewUnsafeValue(entity, meta)

		// 验证各类零值字段获取不报错，且返回对应的零值
		testCases := []struct {
			name    string
			wantVal any
		}{
			{"Id", uint64(0)},
			{"Bool", false},
			{"Int", int(0)},
			{"String", ""},
			{"Float64", float64(0)},
			{"Uint32", uint32(0)},
			{"Byte", byte(0)},
		}

		for _, tc := range testCases {
			got, err := val.GetFieldValue(tc.name)
			assert.NoError(t, err, "字段 %s 的零值不应报错", tc.name)
			assert.Equal(t, tc.wantVal, got, "字段 %s 的零值不匹配", tc.name)
		}
	})

	t.Run("已设置值字段可正常获取", func(t *testing.T) {
		entity := NewSimpleStruct(1)
		val := NewUnsafeValue(entity, meta)
		got, err := val.GetFieldValue("Id")
		assert.NoError(t, err)
		assert.Equal(t, uint64(1), got)
		gotStr, err := val.GetFieldValue("String")
		assert.NoError(t, err)
		assert.Equal(t, "world", gotStr)
	})

	t.Run("未知字段返回错误", func(t *testing.T) {
		entity := &SimpleStruct{}
		val := NewUnsafeValue(entity, meta)
		_, err := val.GetFieldValue("NotExist")
		assert.Error(t, err)
	})
}

func Test_unsafeValue_SetColumns(t *testing.T) {
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

	registry := model.NewRegistry()
	meta, err := registry.Get(&SimpleStruct{})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 通过 sqlmock 构建sql.rows
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = db.Close() }()
			val := NewUnsafeValue(tc.entity, meta)
			cols := make([]string, 0, len(tc.cs))
			colVals := make([]driver.Value, 0, len(tc.cs))
			for k, v := range tc.cs {
				cols = append(cols, k)
				colVals = append(colVals, v)
			}

			mock.ExpectQuery("SELECT *").
				WillReturnRows(sqlmock.NewRows(cols).AddRow(colVals...))

			rows, _ := db.Query("SELECT *")
			if !rows.Next() {
				t.Fatal("没有拿到列")
			}
			err = val.SetColumns(rows)
			if err != nil {
				assert.Equal(t, err, tc.wantVal)
			}
			assert.Equal(t, tc.wantVal, tc.entity)
		})
	}
}
