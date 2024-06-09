package orm

import (
	"errors"
)

type Updater[T any] struct {
	builder
	db      *DB
	assigns []Assignable
	val     *T
	where   []Predicate
}

func NewUpdater[T any](db *DB) *Updater[T] {
	return &Updater[T]{
		builder: builder{
			quoter:  db.dialect.quoter(),
			dialect: db.dialect,
		},
		db: db,
	}
}

func (u *Updater[T]) Build() (*Query, error) {
	var (
		t   T
		err error
	)

	if u.model, err = u.db.r.Get(&t); err != nil {
		return nil, err
	}

	u.sqlStrBuilder.WriteString("UPDATE ")
	u.quote(u.model.TableName)

	// 处理set
	valDealer := u.db.valCreator(u.val, u.model)
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
