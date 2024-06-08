package orm

import (
	"Soil/orm/internal/errs"
	"Soil/orm/internal/model"
	"errors"
	"strings"
)

type builder struct {
	sqlStrBuilder strings.Builder
	args          []any
	model         *model.Model
	quoter        byte
	dialect       Dialect
}

func (b *builder) buildPredicates(ps []Predicate) error {
	p := ps[0]
	for i := 1; i < len(ps); i++ {
		p = p.And(ps[i])
	}

	return b.buildExpression(p)
}

// buildExpression 其实是一个前序遍历(左根右)二叉树的过程
func (b *builder) buildExpression(expr Expression) error {
	if expr == nil {
		return nil
	}

	switch e := expr.(type) {
	case Column:
		// 防止用户在where中使用别名
		e.alias = ""
		return b.buildColumn(e)
	case value:
		b.sqlStrBuilder.WriteByte('?')
		b.args = append(b.args, e.val)
	case Aggregate:
		return b.buildAggregate(e)
	case Predicate:
		//处理左节点
		_, isPredicate := e.left.(Predicate) //类型断言
		if isPredicate {
			b.sqlStrBuilder.WriteByte('(')
		}
		if err := b.buildExpression(e.left); err != nil {
			return err
		}
		if isPredicate {
			b.sqlStrBuilder.WriteByte(')')
		}

		if e.op.String() != "" {
			//处理操作符
			b.sqlStrBuilder.WriteString(" " + e.op.String() + " ")
		}

		//处理右节点
		_, isPredicate = e.left.(Predicate)
		if isPredicate {
			b.sqlStrBuilder.WriteByte('(')
		}
		if err := b.buildExpression(e.right); err != nil {
			return err
		}
		if isPredicate {
			b.sqlStrBuilder.WriteByte(')')
		}
	case RawExpression:
		b.sqlStrBuilder.WriteByte('(')
		b.sqlStrBuilder.WriteString(e.raw)
		b.args = append(b.args, e.args...)
		b.sqlStrBuilder.WriteByte(')')
	default:
		return errors.New("orm: 不支持表达式类型")
	}

	return nil
}

func (b *builder) buildColumn(col Column) error {
	field, ok := b.model.FieldMap[col.name]
	if !ok {
		return errs.NewErrUnknownField(col.name)
	}
	b.quote(field.ColName)
	if col.alias != "" {
		b.sqlStrBuilder.WriteString(" AS ")
		b.quote(col.alias)
	}

	return nil
}

func (b *builder) buildAggregate(a Aggregate) error {
	b.sqlStrBuilder.WriteString(a.fn)
	b.sqlStrBuilder.WriteByte('(')
	if err := b.buildColumn(Column{name: a.arg}); err != nil {
		return err
	}
	b.sqlStrBuilder.WriteByte(')')
	if a.alias != "" {
		b.sqlStrBuilder.WriteString(" AS ")
		b.quote(a.alias)
	}

	return nil
}

func (b *builder) quote(name string) {
	b.sqlStrBuilder.WriteByte(b.quoter)
	b.sqlStrBuilder.WriteString(name)
	b.sqlStrBuilder.WriteByte(b.quoter)
}
