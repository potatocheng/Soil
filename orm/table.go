package orm

type TableReference interface {
	table()
}

type JoinBuilder struct {
	left  TableReference
	typ   string
	right TableReference
}

// Table 普通表
type Table struct {
	entity any
	alias  string
}

func TableOf(entity any) Table {
	return Table{entity: entity}
}

func (t Table) table() {}

func (t Table) Join(right TableReference) *JoinBuilder {
	return &JoinBuilder{
		left:  t,
		right: right,
		typ:   "JOIN",
	}
}

func (t Table) LeftJoin(right TableReference) *JoinBuilder {
	return &JoinBuilder{
		left:  t,
		right: right,
		typ:   "LEFT JOIN",
	}
}

func (t Table) RightJoin(right TableReference) *JoinBuilder {
	return &JoinBuilder{
		left:  t,
		right: right,
		typ:   "RIGHT JOIN",
	}
}

func (t Table) Col(col string) Column {
	return Column{
		name:  col,
		table: t,
	}
}

func (t Table) As(alias string) Table {
	t.alias = alias
	return t
}

type Join struct {
	left  TableReference
	typ   string
	right TableReference
	on    []Predicate
	using []string
}

func (j Join) table() {}

func (j Join) Join(right TableReference) *JoinBuilder {
	return &JoinBuilder{
		left:  j,
		right: right,
		typ:   "JOIN",
	}
}

func (j Join) LeftJoin(right TableReference) *JoinBuilder {
	return &JoinBuilder{
		left:  j,
		right: right,
		typ:   "LEFT JOIN",
	}
}

func (j Join) RightJoin(right TableReference) *JoinBuilder {
	return &JoinBuilder{
		left:  j,
		right: right,
		typ:   "RIGHT JOIN",
	}
}

func (j *JoinBuilder) On(ps ...Predicate) Join {
	return Join{
		left:  j.left,
		typ:   j.typ,
		right: j.right,
		on:    ps,
	}
}

func (j *JoinBuilder) Using(usg ...string) Join {
	return Join{
		left:  j.left,
		right: j.right,
		typ:   j.typ,
		using: usg,
	}
}
