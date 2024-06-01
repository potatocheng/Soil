package orm

type op string

const (
	opEQ  = "="
	opAnd = "AND"
	opOr  = "OR"
	opNOT = "NOT"
)

func (o op) String() string {
	return string(o)
}

// Expression 是标记接口,结构体实现了expr函数，就是一个expression
type Expression interface {
	expr()
}

func exprOf(e any) Expression {
	switch exp := e.(type) {
	case Expression:
		return exp
	default:
		return valueOf(exp)
	}
}

type Predicate struct {
	left  Expression
	op    op
	right Expression
}

func (Predicate) expr() {}

// Not e.g. Not(Col("Age").Eq(18)) == age = 18
func Not(p Predicate) Predicate {
	return Predicate{
		op:    opNOT,
		right: p,
	}
}

// And e.g. Col("Age").Eq(18).AND(Col("ID").EQ(1))--> Age = 18 AND ID = 1
func (l Predicate) And(r Predicate) Predicate {
	return Predicate{
		left:  l,
		op:    opAnd,
		right: r,
	}
}

func (l Predicate) Or(r Predicate) Predicate {
	return Predicate{
		left:  l,
		op:    opOr,
		right: r,
	}
}
