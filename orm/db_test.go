package orm

import (
	"Soil/orm/internal/model"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"reflect"
	"testing"
)

func Test_DB(t *testing.T) {
	type TestModel struct {
		Id        int64
		FirstName string
		Age       int8
		LastName  string
	}

	testCases := []struct {
		name      string
		val       any
		wantModel *model.Model
		wantErr   error
		fieldPtr  *model.Field
	}{
		{
			// 指针
			name: "pointer",
			val:  &TestModel{},
			wantModel: &model.Model{
				TableName: "test_model",
				FieldMap: map[string]*model.Field{
					"Id": {
						ColName: "id",
						Type:    reflect.TypeOf(int64(0)),
						GoName:  "Id",
						Offset:  0,
					},
					"FirstName": {
						ColName: "first_name",
						Type:    reflect.TypeOf(""),
						GoName:  "FirstName",
						Offset:  8,
					},
					"Age": {
						ColName: "age",
						Type:    reflect.TypeOf(int8(0)),
						GoName:  "Age",
						Offset:  24,
					},
					"LastName": {
						ColName: "last_name",
						Type:    reflect.TypeOf(""),
						GoName:  "LastName",
						Offset:  32,
					},
				},
				ColumnMap: map[string]*model.Field{
					"id": {
						ColName: "id",
						Type:    reflect.TypeOf(int64(0)),
						GoName:  "Id",
						Offset:  0,
					},
					"first_name": {
						ColName: "first_name",
						Type:    reflect.TypeOf(""),
						GoName:  "FirstName",
						Offset:  8,
					},
					"age": {
						ColName: "age",
						Type:    reflect.TypeOf(int8(0)),
						GoName:  "Age",
						Offset:  24,
					},
					"last_name": {
						ColName: "last_name",
						Type:    reflect.TypeOf(""),
						GoName:  "LastName",
						Offset:  32,
					},
				},
			},
		},
		{
			name: "test Model",
			val:  &TestModel{},
			wantModel: &model.Model{
				TableName: "test_model",
				FieldMap: map[string]*model.Field{
					"Id":        {ColName: "id", Type: reflect.TypeOf(int64(0)), GoName: "Id", Offset: 0},
					"FirstName": {ColName: "first_name", Type: reflect.TypeOf(""), GoName: "FirstName", Offset: 8},
					"Age":       {ColName: "age", Type: reflect.TypeOf(int8(0)), GoName: "Age", Offset: 24},
					"LastName":  {ColName: "last_name", Type: reflect.TypeOf(""), GoName: "LastName", Offset: 32},
				},
				ColumnMap: map[string]*model.Field{
					"id":         {ColName: "id", Type: reflect.TypeOf(int64(0)), GoName: "Id", Offset: 0},
					"first_name": {ColName: "first_name", Type: reflect.TypeOf(""), GoName: "FirstName", Offset: 8},
					"age":        {ColName: "age", Type: reflect.TypeOf(int8(0)), GoName: "Age", Offset: 24},
					"last_name":  {ColName: "last_name", Type: reflect.TypeOf(""), GoName: "LastName", Offset: 32},
				},
			},
		},
		{
			name: "column tag",
			// 函数最后有()立即调用函数表达式（IIFE）
			val: func() any {
				type ColumnTag struct {
					ID uint64 `orm:"column(id_t)"`
				}
				return &ColumnTag{}
			}(),
			wantModel: &model.Model{
				TableName: "column_tag",
				FieldMap: map[string]*model.Field{
					"ID": &model.Field{ColName: "id_t", Type: reflect.TypeOf(uint64(0)), GoName: "ID", Offset: 0},
				},
				ColumnMap: map[string]*model.Field{
					"id_t": &model.Field{ColName: "id_t", Type: reflect.TypeOf(uint64(0)), GoName: "ID", Offset: 0},
				},
			},
		},
		{
			name: "interface table name",
			val: &CustomTableName{
				FirstName: "firstname",
			},
			wantModel: &model.Model{
				TableName: "test_custom_table_name_t",
				FieldMap: map[string]*model.Field{
					"FirstName": &model.Field{ColName: "first_name", Type: reflect.TypeOf(""), GoName: "FirstName", Offset: 0},
				},
				ColumnMap: map[string]*model.Field{
					"first_name": &model.Field{ColName: "first_name", Type: reflect.TypeOf(""), GoName: "FirstName", Offset: 0},
				},
			},
		},
	}

	mockDB, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	db, err := OpenDB(mockDB)
	if err != nil {
		t.Fatal(err)
	}
	//r := db.r.(*registry)
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			m, err := db.r.Get(testCase.val)
			assert.Equal(t, testCase.wantErr, err)
			if err != nil {
				return
			}

			assert.Equal(t, testCase.wantModel, m)
		})
	}
}

type CustomTableName struct {
	FirstName string
}

func (n *CustomTableName) TableName() string {
	return "test_custom_table_name_t"
}

func Test_With_XXX(t *testing.T) {
	testCases := []struct {
		name      string
		val       any
		opts      []model.ModelOpt
		wantModel *model.Model
		wantErr   error
	}{
		{
			name: "test WithTableName and WithColumnName",
			val:  &TestModel{},
			opts: []model.ModelOpt{
				model.WithTableName("with_table_name_test"),
				model.WithColumName("Id", "id_user"),
			},
			wantModel: &model.Model{
				TableName: "with_table_name_test",
				FieldMap: map[string]*model.Field{
					"Id":        {ColName: "id_user", Type: reflect.TypeOf(int64(0)), GoName: "Id", Offset: 0},
					"FirstName": {ColName: "first_name", Type: reflect.TypeOf(""), GoName: "FirstName", Offset: 8},
					"Age":       {ColName: "age", Type: reflect.TypeOf(uint8(0)), GoName: "Age", Offset: 24},
					"LastName":  {ColName: "last_name", Type: reflect.TypeOf(""), GoName: "LastName", Offset: 32},
				},
				ColumnMap: map[string]*model.Field{
					"id_user":    {ColName: "id_user", Type: reflect.TypeOf(int64(0)), GoName: "Id", Offset: 0},
					"first_name": {ColName: "first_name", Type: reflect.TypeOf(""), GoName: "FirstName", Offset: 8},
					"age":        {ColName: "age", Type: reflect.TypeOf(uint8(0)), GoName: "Age", Offset: 24},
					"last_name":  {ColName: "last_name", Type: reflect.TypeOf(""), GoName: "LastName", Offset: 32},
				},
			},
		},
	}

	mockDB, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	db, err := OpenDB(mockDB)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m, err := db.r.Registry(tc.val, tc.opts...)
			assert.Equal(t, tc.wantErr, err)
			if err != nil {
				return
			}
			//for k, v := range tc.wantModel.FieldMap {
			//	mv, ok := m.FieldMap[k]
			//	if !ok {
			//		t.Fatalf("wantModelFieldMap的key(%s)在返回的model里没有匹配的value", k)
			//	}
			//	if v.ColName != mv.ColName {
			//		t.Fatalf("wantModel.FieldMap<%s, %s>和m.FieldMap<%s, %s>的ColName不相同", k, v.ColName, k, mv.ColName)
			//	}
			//	if v.GoName != mv.GoName {
			//		t.Fatalf("wantModel.FieldMap<%s, %s>和m.FieldMap<%s, %s>的GoName不相同", k, v.GoName, k, mv.GoName)
			//	}
			//	assert.Equal(t, v.Type, mv.Type)
			//}
			//assert.Equal(t, tc.wantModel.FieldMap, m.FieldMap)
			assert.Equal(t, tc.wantModel.ColumnMap, m.ColumnMap)
		})
	}
}
