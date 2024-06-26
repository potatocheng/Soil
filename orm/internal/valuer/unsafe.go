package valuer

import (
	"Soil/orm/internal/errs"
	"Soil/orm/internal/model"
	"database/sql"
	"fmt"
	"reflect"
	"unsafe"
)

type unsafeValue struct {
	addr unsafe.Pointer
	meta *model.Model
}

var _ Creator = NewUnsafeValue

// NewUnsafeValue val只能接收指针
func NewUnsafeValue(val any, meta *model.Model) Valuer {
	return unsafeValue{
		addr: reflect.ValueOf(val).UnsafePointer(),
		meta: meta,
	}
}

func (u unsafeValue) SetColumns(rows *sql.Rows) error {
	colNames, err := rows.Columns()
	if err != nil {
		return err
	}

	if len(colNames) > len(u.meta.ColumnMap) {
		return errs.ErrTooManyReturnedColumns
	}

	colValues := make([]any, len(colNames))
	for i, colName := range colNames {
		column, ok := u.meta.ColumnMap[colName]
		if !ok {
			return errs.NewErrUnknownColumn(colName)
		}

		// NewAt返回的是指针
		val := reflect.NewAt(column.Type, unsafe.Pointer(uintptr(u.addr)+column.Offset))
		colValues[i] = val.Interface()
	}

	// 应为这里这里的val指向的是字段地址在写入后内容直接在各字段内存中
	return rows.Scan(colValues...)
}

func (u unsafeValue) GetFieldValue(name string) (any, error) {
	field, ok := u.meta.FieldMap[name]
	if !ok {
		return nil, errs.NewErrUnknownField(name)
	}
	res := reflect.NewAt(field.Type, unsafe.Pointer(uintptr(u.addr)+field.Offset)).Elem()
	if res.IsZero() {
		return nil, fmt.Errorf("orm: %s 没有设置值(可以使用Set指定设置了值待修改的列)", name)
	}
	return res.Interface(), nil
}
