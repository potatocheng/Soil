package valuer

import (
	"Soil/orm/internal/errs"
	"Soil/orm/internal/model"
	"database/sql"
	"reflect"
)

type reflectValue struct {
	val  reflect.Value
	meta *model.Model
}

// 接口的静态检查， 函数类型的签名是由其参数类型和返回值类型决定的(和C++一样)
var _ Creator = NewReflectValue

// NewReflectValue val只接收指针
func NewReflectValue(val any, meta *model.Model) Valuer {
	return reflectValue{
		val:  reflect.ValueOf(val).Elem(),
		meta: meta,
	}
}

func (r reflectValue) SetColumns(rows *sql.Rows) error {
	columns, err := rows.Columns()
	if err != nil {
		return err
	}

	vals := make([]any, len(columns))
	for i, column := range columns {
		fd, ok := r.meta.ColumnMap[column]
		if !ok {
			return errs.NewErrUnknownColumn(column)
		}
		newVal := reflect.New(fd.Type)
		vals[i] = newVal.Interface()
	}

	err = rows.Scan(vals...)
	if err != nil {
		return err
	}

	for i, column := range columns {
		fd, ok := r.meta.ColumnMap[column]
		if !ok {
			return errs.NewErrUnknownColumn(column)
		}
		r.val.FieldByName(fd.GoName).Set(reflect.ValueOf(vals[i]).Elem())
	}

	return nil
}
