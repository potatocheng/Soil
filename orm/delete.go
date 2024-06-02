package orm

import model2 "Soil/orm/internal/model"

type Deleter[T any] struct {
	builder
	tableName string
	where     []Predicate

	model *model2.Model
	db    *DB
}

func (d *Deleter[T]) Build() (*Query, error) {
	var (
		t   T
		err error
	)

	d.model, err = d.db.r.Get(&t)
	if err != nil {
		return nil, err
	}

	d.sqlStrBuilder.WriteString("DELETE FROM")

	//处理FROM
	if d.tableName != "" {
		// 用户没有调用From, 直接使用泛型的类型名
		d.sqlStrBuilder.WriteByte('`')
		d.sqlStrBuilder.WriteString(d.model.TableName)
		d.sqlStrBuilder.WriteByte('`')
	} else {
		//用户调用了From, 用户传入什么就使用什么
		d.sqlStrBuilder.WriteString(d.tableName)
	}

	//处理WHERE
	if len(d.where) > 0 {
		d.sqlStrBuilder.WriteString(" WHERE ")
		if err = d.buildPredicates(d.where); err != nil {
			return nil, err
		}
	}

	d.sqlStrBuilder.WriteByte(';')
	return &Query{
		SQL:  d.sqlStrBuilder.String(),
		Args: d.args,
	}, nil
}

func (d *Deleter[T]) From(tableName string) *Deleter[T] {
	d.tableName = tableName

	return d
}

// Where e.g. where(Col("id").EQ(1))
func (d *Deleter[T]) Where(p ...Predicate) *Deleter[T] {
	d.where = p

	return d
}

func NewDeleter[T any](db *DB) *Deleter[T] {
	return &Deleter[T]{
		db: db,
	}
}
