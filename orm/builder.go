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

	model *model.Model
}

func (b *builder) buildPredicates(ps []Predicate) error {
	p := ps[0]
	for i := 1; i < len(ps); i++ {
		p = p.And(ps[i])
	}

	if err := b.buildExpression(p); err != nil {
		return err
	}

	return nil
}

// buildExpression 其实是一个前序遍历(左根右)二叉树的过程
func (b *builder) buildExpression(expr Expression) error {
	if expr == nil {
		return nil
	}

	switch e := expr.(type) {
	case Column:
		field, ok := b.model.FieldMap[e.name]
		if !ok {
			return errs.NewErrUnknownField(e.name)
		}
		b.sqlStrBuilder.WriteByte('`')
		b.sqlStrBuilder.WriteString(field.ColName)
		b.sqlStrBuilder.WriteByte('`')
	case value:
		b.sqlStrBuilder.WriteByte('?')
		b.args = append(b.args, e.val)
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

		//处理操作符
		b.sqlStrBuilder.WriteString(" " + e.op.String() + " ")

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
	default:
		return errors.New("orm: 不支持表达式类型")
	}

	return nil
}
