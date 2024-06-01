package orm

type Column struct {
	name string
}

func (c Column) expr() {}

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
