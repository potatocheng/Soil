package orm

import (
	"context"
	"database/sql"
	"errors"
)

type Updater[T any] struct {
	builder
	assigns []Assignable
	val     *T
	where   []Predicate

	session Session
}

func NewUpdater[T any](session Session) *Updater[T] {
	c := session.getCore()
	return &Updater[T]{
		session: session,
		builder: builder{
			core:   c,
			quoter: c.dialect.quoter(),
		},
	}
}

func (u *Updater[T]) Build() (*Query, error) {
	var (
		t   T
		err error
	)

	if u.model, err = u.r.Get(&t); err != nil {
		return nil, err
	}

	u.sqlStrBuilder.WriteString("UPDATE ")
	u.quote(u.model.TableName)

	// 处理set
	valDealer := u.valCreator(u.val, u.model)
	if len(u.assigns) > 0 {
		u.sqlStrBuilder.WriteString(" SET ")
		for idx, assign := range u.assigns {
			if idx > 0 {
				u.sqlStrBuilder.WriteString(",")
			}
			switch assign := assign.(type) {
			case Assignment:
				if err = u.buildAssignment(assign); err != nil {
					return nil, err
				}
			case Column:
				var res any
				res, err = valDealer.GetFieldValue(assign.name)
				if err != nil {
					return nil, err
				}
				err = u.buildColumn(Col(assign.name))
				if err != nil {
					return nil, err
				}
				u.sqlStrBuilder.WriteString("=?")
				u.args = append(u.args, res)
			}
		}
	} else {
		if u.val == nil {
			return nil, errors.New("orm: update 没有设置条件")
		}
		//NewUpdater[User].Update(&User)--->UPDATE `user` SET (User里的字段)
		u.sqlStrBuilder.WriteString(" SET ")
		for idx, field := range u.model.Fields {
			if idx > 0 {
				u.sqlStrBuilder.WriteString(",")
			}
			if err = u.buildColumn(Col(field.GoName)); err != nil {
				return nil, err
			}
			u.sqlStrBuilder.WriteString("=?")
			var v any
			v, err = valDealer.GetFieldValue(field.GoName)
			if err != nil {
				return nil, err
			}
			u.args = append(u.args, v)
		}
	}

	// 处理where
	if len(u.where) > 0 {
		u.sqlStrBuilder.WriteString(" WHERE ")

		err = u.buildPredicates(u.where)
		if err != nil {
			return nil, err
		}
	}

	u.sqlStrBuilder.WriteByte(';')
	return &Query{
		SQL:  u.sqlStrBuilder.String(),
		Args: u.args,
	}, nil
}

func (u *Updater[T]) Set(assigns ...Assignable) *Updater[T] {
	u.assigns = assigns

	return u
}

func (u *Updater[T]) Update(val *T) *Updater[T] {
	u.val = val
	return u
}

func (u *Updater[T]) Where(where ...Predicate) *Updater[T] {
	u.where = where
	return u
}

func (u *Updater[T]) Exec(ctx context.Context) Result {
	var err error
	u.model, err = u.r.Get(new(T))
	if err != nil {
		return Result{err: err}
	}

	res := exec(ctx, u.core, u.session, &QueryContext{
		Type:         "UPDATE",
		QueryBuilder: u,
		Model:        u.model,
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
