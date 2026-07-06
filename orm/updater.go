package orm

import (
	"Soil/orm/internal/errs"
	"Soil/orm/internal/model"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
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
		// 乐观锁：在 SET 末尾追加 version=version+1（DB 端原子自增）。
		// 仅当模型定义了 VersionField、持有模型实例（Update(val)）且用户未显式 SET version 列时追加。
		// 批量更新（u.val == nil）场景显式 opt-out，不追加。
		if u.model.VersionField != nil && u.val != nil && !u.versionColumnExplicitlySet() {
			u.sqlStrBuilder.WriteString(",")
			u.quote(u.model.VersionField.ColName)
			u.sqlStrBuilder.WriteString("=")
			u.quote(u.model.VersionField.ColName)
			u.sqlStrBuilder.WriteString("+1")
		}
	} else {
		if u.val == nil {
			return nil, errors.New("orm: update 没有设置条件")
		}
		//NewUpdater[User].Update(&User)--->UPDATE `user` SET (User里的字段)
		u.sqlStrBuilder.WriteString(" SET ")
		// first 用于正确处理逗号：当 version 字段被跳过时，下一个字段不应产生前导逗号。
		first := true
		for _, field := range u.model.Fields {
			// 跳过 version 字段：循环结束后追加 version=version+1，
			// 避免在此处写入 version=<当前值>（与自增语义冲突）。
			if u.model.VersionField != nil && field.GoName == u.model.VersionField.GoName {
				continue
			}
			if !first {
				u.sqlStrBuilder.WriteString(",")
			}
			first = false
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
		// 乐观锁：追加 version=version+1（DB 端原子自增）。
		// 此分支下 u.val 必非 nil（上方已校验），故仅检查 VersionField。
		if u.model.VersionField != nil {
			if !first {
				u.sqlStrBuilder.WriteString(",")
			}
			u.quote(u.model.VersionField.ColName)
			u.sqlStrBuilder.WriteString("=")
			u.quote(u.model.VersionField.ColName)
			u.sqlStrBuilder.WriteString("+1")
		}
	}

	// 处理where（含软删除过滤）
	if err = u.buildWhereWithSoftDelete(u.where); err != nil {
		return nil, err
	}

	// 乐观锁：在 WHERE 末尾追加 AND <version_col>=?，参数为通过反射从 u.val 读取的当前版本值。
	// 仅当模型定义了 VersionField 且持有模型实例（Update(val)）时启用。
	// buildWhereWithSoftDelete 仅在存在软删除字段或用户 WHERE 时输出 " WHERE "，
	// 这里据此决定追加 " AND " 还是 " WHERE "。
	if u.model.VersionField != nil && u.val != nil {
		hasWhere := u.model.DeletedAtField != nil || len(u.where) > 0
		if hasWhere {
			u.sqlStrBuilder.WriteString(" AND ")
		} else {
			u.sqlStrBuilder.WriteString(" WHERE ")
		}
		u.quote(u.model.VersionField.ColName)
		u.sqlStrBuilder.WriteString("=?")
		var verVal any
		verVal, err = readVersionFromVal(u.val, u.model.VersionField)
		if err != nil {
			return nil, err
		}
		u.args = append(u.args, verVal)
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

	// 乐观锁冲突检测：仅当模型定义了 VersionField 且持有模型实例（Update(val)）时启用。
	// RowsAffected()==0 表示 WHERE version=? 未命中任何行（版本已被并发事务修改）。
	// 在 AfterUpdate 钩子之前返回，避免冲突时仍触发后置钩子。
	if res.Error == nil && u.model.VersionField != nil && u.val != nil {
		if sqlRes != nil {
			affected, e := sqlRes.RowsAffected()
			if e == nil && affected == 0 {
				return Result{err: errs.ErrOptimisticLock}
			}
		}
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

// versionColumnExplicitlySet 检查 u.assigns 是否已显式包含对 version 字段的赋值
// （无论是 Assignment 还是 Column 形式）。若是，则 Build 跳过自动追加 version=version+1，
// 与 UpdatedAtField 的检测模式保持一致。
func (u *Updater[T]) versionColumnExplicitlySet() bool {
	if u.model == nil || u.model.VersionField == nil {
		return false
	}
	name := u.model.VersionField.GoName
	for _, a := range u.assigns {
		switch a := a.(type) {
		case Assignment:
			if a.column == name {
				return true
			}
		case Column:
			if a.name == name {
				return true
			}
		}
	}
	return false
}

// readVersionFromVal 通过反射从 val（指向结构体的指针）读取 field 指定的版本字段当前值，返回 int64。
// registry 已保证 VersionField 为整数族类型（int/int8-64、uint/uint8-64），
// 这里仍对 Kind 做分支处理以兼容 int 与 uint 各宽度。
func readVersionFromVal(val any, field *model.Field) (int64, error) {
	v := reflect.ValueOf(val).Elem()
	fv := v.FieldByName(field.GoName)
	if !fv.IsValid() {
		return 0, errs.NewErrUnknownField(field.GoName)
	}
	switch fv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return fv.Int(), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int64(fv.Uint()), nil
	default:
		return 0, fmt.Errorf("orm: 字段 %s 类型 %s 不是整数族，无法作为版本字段", field.GoName, fv.Type())
	}
}
