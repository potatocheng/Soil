package orm

// Expression 是标记接口,结构体实现了expr函数，就是一个expression
type Expression interface {
	expr()
}

// RawExpression 是原生表达式
type RawExpression struct {
	raw  string
	args []any
}

func (r RawExpression) expr()       {}
func (r RawExpression) Selectable() {}

func Raw(raw string, args ...any) RawExpression {
	return RawExpression{raw: raw, args: args}
}

func (r RawExpression) AsPredicate() Predicate {
	return Predicate{
		left: r,
	}
}
