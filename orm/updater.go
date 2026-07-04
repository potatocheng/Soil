package orm

import (
	"context"
	"database/sql"
	"errors"
	"time"
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

	// 处理where（含软删除过滤）
	if err = u.buildWhereWithSoftDelete(u.where); err != nil {
		return nil, err
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

	// BeforeUpdate 钩子：仅当 Updater 通过 Update(val) 持有模型实例时调用。
	// 仅使用 Set(...) 的批量更新场景没有模型实例，跳过钩子。
	if u.val != nil {
		if h, ok := any(u.val).(BeforeUpdate); ok {
			if e := h.BeforeUpdate(ctx); e != nil {
				return Result{err: e}
			}
		}
	}

	// 自动填充 UpdatedAt：在钩子之后、Build 之前执行。
	// - 持有模型实例（Update(val)）时：通过反射设置字段值，Build 从 val 读取。
	// - 仅使用 Set(...) 的批量更新时：将 UpdatedAt 作为额外 Assignment 追加到 assigns。
	// 若用户已显式 Assign/Col 了 UpdatedAt，则不重复追加，避免 SQL 中出现重复列。
	if u.model.UpdatedAtField != nil {
		now := time.Now()
		updatedName := u.model.UpdatedAtField.GoName
		if u.val != nil {
			if e := setTimestampField(u.val, u.model.UpdatedAtField, now); e != nil {
				return Result{err: e}
			}
		}
		if len(u.assigns) > 0 {
			alreadyHas := false
			for _, a := range u.assigns {
				switch a := a.(type) {
				case Assignment:
					if a.column == updatedName {
						alreadyHas = true
					}
				case Column:
					if a.name == updatedName {
						alreadyHas = true
					}
				}
			}
			if !alreadyHas {
				u.assigns = append(u.assigns, Assign(updatedName, now))
			}
		}
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

	// AfterUpdate 钩子：仅在 SQL 执行成功且持有模型实例时调用。
	if res.Error == nil && u.val != nil {
		if h, ok := any(u.val).(AfterUpdate); ok {
			if e := h.AfterUpdate(ctx); e != nil {
				return Result{err: e}
			}
		}
	}

	return Result{
		err: res.Error,
		res: sqlRes,
	}
}
