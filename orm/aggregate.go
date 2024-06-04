package orm

type Aggregate struct {
	fn    string
	arg   string
	alias string
}

func (a Aggregate) Selectable() {}

func Avg(arg string) Aggregate {
	return Aggregate{fn: "AVG", arg: arg}
}

func Count(arg string) Aggregate {
	return Aggregate{fn: "COUNT", arg: arg}
}

func Sum(arg string) Aggregate {
	return Aggregate{fn: "SUM", arg: arg}
}

func Max(arg string) Aggregate {
	return Aggregate{fn: "MAX", arg: arg}
}

func Min(arg string) Aggregate {
	return Aggregate{fn: "MIN", arg: arg}
}

func (a Aggregate) As(alias string) Aggregate {
	a.alias = alias

	return a
}
