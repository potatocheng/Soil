package orm

// Assignable 标记接口，实现该接口，表示可以用于赋值语句，一般用于UPDATE和UPSERT
type Assignable interface {
	assign()
}

type Assignment struct {
	column string
	val    Expression
}

// 实现标记接口
func (a Assignment) assign() {}

func Assign(column string, val any) Assignment {
	return Assignment{
		column: column,
		val:    exprOf(val),
	}
}
