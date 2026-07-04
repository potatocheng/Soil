package orm

import (
	"context"
	"database/sql"
	"time"
)

type Deleter[T any] struct {
	builder
	tableName string
	where     []Predicate

	session Session
}

func (d *Deleter[T]) Build() (*Query, error) {
	var (
		t   T
		err error
	)

	d.model, err = d.r.Get(&t)
	if err != nil {
		return nil, err
	}

	// 软删除：若模型定义了 DeletedAtField，将 DELETE 改写为
	// `UPDATE <table> SET deleted_at=? WHERE deleted_at IS NULL [AND <user where>]`。
	if d.model.DeletedAtField != nil {
		return d.buildSoftDelete()
	}

	d.sqlStrBuilder.WriteString("DELETE FROM ")

	//处理FROM
	if d.tableName == "" {
		// 用户没有调用From, 直接使用泛型的类型名
		d.quote(d.model.TableName)
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

// buildSoftDelete 生成软删除改写后的 UPDATE 语句。
// 将 deleted_at 列设置为当前时间，并通过 buildWhereWithSoftDelete 追加
// `deleted_at IS NULL` 过滤，避免重复软删除已删除的行。
func (d *Deleter[T]) buildSoftDelete() (*Query, error) {
	d.sqlStrBuilder.WriteString("UPDATE ")
	if d.tableName == "" {
		d.quote(d.model.TableName)
	} else {
		d.sqlStrBuilder.WriteString(d.tableName)
	}
	d.sqlStrBuilder.WriteString(" SET ")
	d.quote(d.model.DeletedAtField.ColName)
	d.sqlStrBuilder.WriteString("=?")
	d.args = append(d.args, time.Now())

	if err := d.buildWhereWithSoftDelete(d.where); err != nil {
		return nil, err
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

func NewDeleter[T any](session Session) *Deleter[T] {
	c := session.getCore()
	return &Deleter[T]{
		builder: builder{
			core:   c,
			quoter: c.dialect.quoter(),
		},
		session: session,
	}
}

func (d *Deleter[T]) Exec(ctx context.Context) Result {
	var err error
	d.model, err = d.r.Get(new(T))
	if err != nil {
		return Result{err: err}
	}

	res := exec(ctx, d.core, d.session, &QueryContext{
		Type:         "DELETE",
		QueryBuilder: d,
		Model:        d.model,
	})

	var sqlRes sql.Result
	if res.Result != nil {
		sqlRes = res.Result.(sql.Result)
	}
	return Result{
		err: res.Error,
		res: sqlRes,
	}
}
