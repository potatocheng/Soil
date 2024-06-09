package orm

import (
	"Soil/orm/internal/errs"
	"Soil/orm/internal/model"
	"context"
)

type UpsertBuilder[T any] struct {
	inserter        *Inserter[T]
	conflictColumns []string
}

type Upsert struct {
	conflictColumns []string
	assigns         []Assignable
}

func (s *UpsertBuilder[T]) ConflictColumns(cols ...string) *UpsertBuilder[T] {
	s.conflictColumns = cols
	return s
}

// Update 调用了这个函数后就表示Upsert的数据设置完成，之后调用Insert的函数继续设置Insert的数据
func (s *UpsertBuilder[T]) Update(assigns ...Assignable) *Inserter[T] {
	s.inserter.upsert = &Upsert{
		conflictColumns: s.conflictColumns,
		assigns:         assigns,
	}

	return s.inserter
}

type Inserter[T any] struct {
	builder
	values  []*T
	columns []string
	db      *DB

	upsert *Upsert
}

func NewInserter[T any](db *DB) *Inserter[T] {
	return &Inserter[T]{
		builder: builder{
			quoter:  db.dialect.quoter(),
			dialect: db.dialect,
		},
		db: db,
	}
}

func (i *Inserter[T]) OnDuplicateKey() *UpsertBuilder[T] {
	return &UpsertBuilder[T]{
		inserter: i,
	}
}

func (i *Inserter[T]) Values(val ...*T) *Inserter[T] {
	i.values = append(i.values, val...)

	return i
}

func (i *Inserter[T]) Columns(col ...string) *Inserter[T] {
	i.columns = append(i.columns, col...)

	return i
}

func (i *Inserter[T]) Build() (*Query, error) {
	if len(i.values) == 0 {
		return nil, errs.ErrInsertZeroRow
	}

	var err error
	// 获得元数据
	i.model, err = i.db.r.Get(new(T))
	if err != nil {
		return nil, err
	}

	i.sqlStrBuilder.WriteString("INSERT INTO ")
	i.quote(i.model.TableName)
	i.sqlStrBuilder.WriteByte('(')

	// 获取列名
	fields := i.model.Fields
	if len(i.columns) != 0 {
		//用户指定列名
		fields = make([]*model.Field, 0, len(i.columns))
		for _, col := range i.columns {
			field, ok := i.model.FieldMap[col]
			if !ok {
				return nil, errs.NewErrUnknownField(col)
			}

			fields = append(fields, field)
		}
	}

	//for k, v := range meta.FieldMap //这样不行，因为每次遍历k-v对顺序不同
	for idx, field := range fields {
		if idx != 0 {
			i.sqlStrBuilder.WriteByte(',')
		}
		i.quote(field.ColName)
	}

	i.sqlStrBuilder.WriteByte(')')

	// 处理VALUES部分,处理参数
	i.sqlStrBuilder.WriteString(" VALUES ")
	i.args = make([]any, 0, len(fields)*len(i.values)+1)
	for j, val := range i.values {
		if j != 0 {
			i.sqlStrBuilder.WriteByte(',')
		}
		valDealer := i.db.valCreator(val, i.model)
		i.sqlStrBuilder.WriteByte('(')
		for idx, field := range fields {
			if idx != 0 {
				i.sqlStrBuilder.WriteByte(',')
			}
			i.sqlStrBuilder.WriteByte('?')
			fdVal, err := valDealer.GetFieldValue(field.GoName)
			if err != nil {
				return nil, err
			}
			i.args = append(i.args, fdVal)
		}
		i.sqlStrBuilder.WriteByte(')')
	}

	// 处理Upsert部分
	if i.upsert != nil {
		err = i.dialect.buildUpsert(&(i.builder), i.upsert)
		if err != nil {
			return nil, err
		}
	}

	i.sqlStrBuilder.WriteByte(';')
	return &Query{
		SQL:  i.sqlStrBuilder.String(),
		Args: i.args,
	}, nil
}

//	func (i *Inserter[T]) buildAssigment(a Assignable) error {
//		switch assign := a.(type) {
//		case Assignment:
//			i.sqlStrBuilder.WriteByte('`')
//			field, ok := i.model.FieldMap[assign.column]
//			if !ok {
//				return errs.NewErrUnknownField(assign.column)
//			}
//			i.sqlStrBuilder.WriteString(field.ColName)
//			i.sqlStrBuilder.WriteByte('`')
//			i.sqlStrBuilder.WriteString("=?")
//			i.args = append(i.args, assign.val)
//		case Column:
//			field, ok := i.model.FieldMap[assign.name]
//			if !ok {
//				return errs.NewErrUnknownField(assign.name)
//			}
//			i.sqlStrBuilder.WriteByte('`')
//			i.sqlStrBuilder.WriteString(field.ColName)
//			i.sqlStrBuilder.WriteString("`=VALUES(`")
//			i.sqlStrBuilder.WriteString(field.ColName)
//			i.sqlStrBuilder.WriteString("`)")
//		}
//
//		return nil
//	}
func (i *Inserter[T]) Exec(ctx context.Context) Result {
	query, err := i.Build()
	if err != nil {
		return Result{
			err: err,
		}
	}
	res, err := i.db.db.ExecContext(ctx, query.SQL, query.Args...)
	return Result{res: res, err: err}
}
