package orm

import (
	"Soil/orm/internal/errs"
	"context"
	"reflect"
	"unsafe"
)

type Selector[T any] struct {
	builder
	table string
	where []Predicate

	model *Model
	db    *DB
}

// Build 生成sql语句和获得参数
func (s *Selector[T]) Build() (*Query, error) {
	var (
		t   T
		err error
	)

	s.model, err = s.db.r.Get(&t)
	if err != nil {
		return nil, err
	}
	s.sqlStrBuilder.WriteString("SELECT * FROM ")

	// 处理from
	if s.table == "" {
		//没有调用From，那么table就是T的类型名
		s.sqlStrBuilder.WriteByte('`')
		s.sqlStrBuilder.WriteString(s.model.TableName)
		s.sqlStrBuilder.WriteByte('`')
	} else {
		//调用了From，初始化了table
		s.sqlStrBuilder.WriteString(s.table)
	}

	// 处理where之后的条件
	if len(s.where) > 0 {
		s.sqlStrBuilder.WriteString(" WHERE ")
		err := s.buildPredicates(s.where)
		if err != nil {
			return nil, err
		}
	}

	s.sqlStrBuilder.WriteByte(';')
	return &Query{
		SQL:  s.sqlStrBuilder.String(),
		Args: s.args,
	}, nil
}

func (s *Selector[T]) From(tbl string) *Selector[T] {
	s.table = tbl

	return s
}

func (s *Selector[T]) Where(p ...Predicate) *Selector[T] {
	s.where = p

	return s
}

func (s *Selector[T]) GetV1(ctx context.Context) (*T, error) {
	query, err := s.Build()
	if err != nil {
		return nil, err
	}

	rows, err := s.db.db.QueryContext(ctx, query.SQL, query.Args...)
	if err != nil {
		return nil, err
	}

	if !rows.Next() {
		return nil, errs.ErrNoRows
	}

	//获得数据库返回的列信息
	colNames, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	// 获得元数据
	retValuePtr := new(T)
	meta, err := s.db.r.Get(retValuePtr)
	if err != nil {
		return nil, err
	}

	var vals []any
	for _, colName := range colNames {
		field, ok := meta.ColumnMap[colName]
		if !ok {
			return nil, errs.NewErrUnknownColumn(colName)
		}

		// NewAt返回的是指针
		val := reflect.NewAt(field.Type, unsafe.Pointer(uintptr(reflect.ValueOf(retValuePtr).UnsafePointer())+field.Offset))
		vals = append(vals, val.Interface())
	}

	// 应为这里这里的val指向的是字段地址在写入后内容直接在各字段内存中
	err = rows.Scan(vals...)
	if err != nil {
		return nil, err
	}

	return retValuePtr, nil
}

// Get 获得数据库数据，将数据转为go结构体返回
func (s *Selector[T]) Get(ctx context.Context) (*T, error) {
	query, err := s.Build()
	if err != nil {
		return nil, err
	}

	//执行sql查询语句,
	rows, err := s.db.db.QueryContext(ctx, query.SQL, query.Args...)
	if err != nil {
		return nil, err
	}

	if !rows.Next() {
		return nil, errs.ErrNoRows
	}

	//将sql数据通过反射转换为go类型
	columnNames, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	vals := make([]any, 0, len(columnNames))
	retVal := new(T)

	model, err := s.db.r.Get(retVal)
	if err != nil {
		return nil, err
	}
	//初始化vals的具体go类型
	//for _, colName := range columnNames {
	//	for _, v := range model.FieldMap {
	//		if colName == v.ColName {
	//			val := reflect.New(v.Type)
	//			vals = append(vals, val.Interface())
	//		}
	//	}
	//}
	for _, colName := range columnNames {
		field, ok := model.ColumnMap[colName]
		if !ok {
			return nil, errs.NewErrUnknownColumn(colName)
		}
		// New也是返回一个指向该类型的指针，如果要获得这个值要调用Elem()
		val := reflect.New(field.Type)
		vals = append(vals, val.Interface())
	}

	//这里获得vals, vals里存储的是指针
	err = rows.Scan(vals...)
	if err != nil {
		return nil, err
	}

	//将vals中的数据赋值给结构体t
	refRetVal := reflect.ValueOf(retVal)
	//for i, colName := range columnNames {
	//	for _, v := range model.FieldMap {
	//		if colName == v.ColName {
	//			tValue.Elem().FieldByName(v.GoName).Set(reflect.ValueOf(vals[i]).Elem())
	//		}
	//	}
	//}
	for i, colName := range columnNames {
		field, ok := model.ColumnMap[colName]
		if !ok {
			return nil, errs.NewErrUnknownColumn(colName)
		}
		refRetVal.Elem().FieldByName(field.GoName).Set(reflect.ValueOf(vals[i]).Elem())
	}

	return retVal, nil
}

func NewSelector[T any](db *DB) *Selector[T] {
	return &Selector[T]{
		db: db,
	}
}
