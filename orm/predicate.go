package orm

type op string

const (
	opEQ    = "="
	opAnd   = "AND"
	opOr    = "OR"
	opNOT   = "NOT"
	opLT    = "<"
	opGT    = ">"
	opAdd   = "+"
	opSub   = "-"
	opMulti = "*"
	opDiv   = "/"
)

func (o op) String() string {
	return string(o)
}

func exprOf(e any) Expression {
	switch exp := e.(type) {
	case Expression: // 这个case目前是在形参为RawExpression时走这里
		return exp
	default:
		return valueOf(exp)
	}
}

type Predicate binaryExpression

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
