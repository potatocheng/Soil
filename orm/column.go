package orm

type Column struct {
	name  string
	table TableReference
	alias string
}

func (c Column) expr()       {}
func (c Column) Selectable() {}
func (c Column) assign()     {}

func (c Column) As(alias string) Column {
	c.alias = alias

	return c
}

type value struct {
	val any
}

func (v value) expr() {}

func valueOf(val any) value {
	return value{val: val}
}

func Col(name string) Column {
	return Column{name: name}
}

func (c Column) EQ(arg any) Predicate {
	return Predicate{
		left:  c,
		op:    opEQ,
		right: exprOf(arg),
	}
}

func (c Column) GT(arg any) Predicate {
	return Predicate{
		left:  c,
		op:    opGT,
		right: exprOf(arg),
	}
}

func (c Column) LT(arg any) Predicate {
	return Predicate{
		left:  c,
		op:    opLT,
		right: exprOf(arg),
	}
}
