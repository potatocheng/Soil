package orm

import (
	"Soil/orm/internal/errs"
	"context"
)

type Selector[T any] struct {
	builder
	table   TableReference
	where   []Predicate
	columns []Selectable
	groupBy []Column
	having  []Predicate
	orderBy []OrderBy
	offset  int
	limit   int

	session Session
}

// Build 生成sql语句和获得参数
func (s *Selector[T]) Build() (*Query, error) {
	var (
		t   T
		err error
	)

	s.model, err = s.r.Get(&t)
	if err != nil {
		return nil, err
	}

	s.sqlStrBuilder.WriteString("SELECT ")

	// 处理SELECT后面跟着的列
	if err = s.buildColumns(); err != nil {
		return nil, err
	}

	// 处理from
	s.sqlStrBuilder.WriteString(" FROM ")
	err = s.buildTable(s.table)
	if err != nil {
		return nil, err
	}

	// 处理where之后的条件
	if len(s.where) > 0 {
		s.sqlStrBuilder.WriteString(" WHERE ")
		err := s.buildPredicates(s.where)
		if err != nil {
			return nil, err
		}
	}

	// 处理GroupBy数据
	if len(s.groupBy) > 0 {
		s.sqlStrBuilder.WriteString(" GROUP BY ")
		for idx, groupCol := range s.groupBy {
			if idx > 0 {
				s.sqlStrBuilder.WriteByte(',')
			}
			if err = s.buildColumn(groupCol); err != nil {
				return nil, err
			}
		}
	}

	// 处理Having条件
	if len(s.having) > 0 {
		s.sqlStrBuilder.WriteString(" HAVING ")
		if err = s.buildPredicates(s.having); err != nil {
			return nil, err
		}
	}

	// 处理Order BY
	if len(s.orderBy) > 0 {
		s.sqlStrBuilder.WriteString(" ORDER BY ")
		if err = s.buildOrderBy(); err != nil {
			return nil, err
		}
	}

	if s.limit > 0 {
		s.sqlStrBuilder.WriteString(" LIMIT ?")
		s.args = append(s.args, s.limit)
	}

	if s.offset > 0 {
		s.sqlStrBuilder.WriteString(" OFFSET ?")
		s.args = append(s.args, s.offset)
	}

	s.sqlStrBuilder.WriteByte(';')
	return &Query{
		SQL:  s.sqlStrBuilder.String(),
		Args: s.args,
	}, nil
}

func (s *Selector[T]) From(tbl TableReference) *Selector[T] {
	s.table = tbl

	return s
}

func (s *Selector[T]) Where(p ...Predicate) *Selector[T] {
	s.where = p

	return s
}

// Select 参数传入结构体字段名
func (s *Selector[T]) Select(cols ...Selectable) *Selector[T] {
	s.columns = cols

	return s
}

func (s *Selector[T]) Get(ctx context.Context) (*T, error) {
	var err error
	s.model, err = s.r.Get(new(T))
	if err != nil {
		return nil, err
	}
	res := get[T](ctx, s.session, s.core, &QueryContext{
		Type:         "SELECT",
		QueryBuilder: s,
		Model:        s.model,
	})
	if res.Result != nil {
		return res.Result.(*T), res.Error
	}
	return nil, res.Error
}

func get[T any](ctx context.Context, session Session, c core, qc *QueryContext) *QueryResult {
	var root Handler = func(ctx context.Context, queryCtx *QueryContext) *QueryResult {
		return getHandler[T](ctx, session, c, qc)
	}

	for i := len(c.middlewares) - 1; i >= 0; i-- {
		root = c.middlewares[i](root)
	}

	return root(ctx, qc)
}

func getHandler[T any](ctx context.Context, session Session, c core, qc *QueryContext) *QueryResult {
	query, err := qc.QueryBuilder.Build()
	if err != nil {
		return &QueryResult{Error: err}
	}

	//执行sql查询语句,
	rows, err := session.queryContext(ctx, query.SQL, query.Args...)
	if err != nil {
		return &QueryResult{Error: err}
	}

	if !rows.Next() {
		return &QueryResult{Error: errs.ErrNoRows}
	}

	retVal := new(T)
	valuer := c.valCreator(retVal, c.model)
	err = valuer.SetColumns(rows)

	return &QueryResult{
		Error:  err,
		Result: retVal,
	}
}

func NewSelector[T any](session Session) *Selector[T] {
	c := session.getCore()
	return &Selector[T]{
		session: session,
		builder: builder{
			core:   session.getCore(),
			quoter: c.dialect.quoter(),
		},
	}
}

// Selectable 标记接口，表明这个字段可以作为SELECT XXX 中的内容
type Selectable interface {
	Selectable()
}

func (s *Selector[T]) buildColumns() error {
	if len(s.columns) == 0 {
		s.sqlStrBuilder.WriteByte('*')
		return nil
	}

	for i, col := range s.columns {
		if i > 0 {
			s.sqlStrBuilder.WriteByte(',')
		}
		switch expr := col.(type) {
		case Column:
			if err := s.buildColumn(expr); err != nil {
				return err
			}
		case Aggregate:
			if err := s.buildAggregate(expr); err != nil {
				return err
			}
		case RawExpression:
			s.sqlStrBuilder.WriteString(expr.raw)
			s.args = append(s.args, expr.args...)
		}
	}

	return nil
}

func (s *Selector[T]) GroupBy(column ...Column) *Selector[T] {
	s.groupBy = column

	return s
}

func (s *Selector[T]) Having(p ...Predicate) *Selector[T] {
	s.having = p

	return s
}

type OrderBy struct {
	col   string
	order string
}

func Asc(col string) OrderBy {
	return OrderBy{col: col, order: "ASC"}
}

func Desc(col string) OrderBy {
	return OrderBy{col: col, order: "DESC"}
}

func (s *Selector[T]) OrderBy(OrderBys ...OrderBy) *Selector[T] {
	s.orderBy = OrderBys
	return s
}

func (s *Selector[T]) buildOrderBy() error {
	for idx, o := range s.orderBy {
		if idx > 0 {
			s.sqlStrBuilder.WriteByte(',')
		}
		if err := s.buildColumn(Col(o.col)); err != nil {
			return err
		}
		s.sqlStrBuilder.WriteString(" " + o.order)
	}

	return nil
}

func (s *Selector[T]) Offset(offset int) *Selector[T] {
	s.offset = offset
	return s
}

func (s *Selector[T]) Limit(limit int) *Selector[T] {
	s.limit = limit
	return s
}

func (s *Selector[T]) buildTable(table TableReference) error {
	switch typ := table.(type) {
	case nil:
		s.quote(s.model.TableName)
	case Table:
		meta, err := s.r.Get(typ.entity)
		if err != nil {
			return err
		}
		s.quote(meta.TableName)
		if typ.alias != "" {
			s.sqlStrBuilder.WriteString(" AS ")
			s.quote(typ.alias)
		}
	case Join:
		s.sqlStrBuilder.WriteByte('(')
		// 处理左节点
		if err := s.buildTable(typ.left); err != nil {
			return err
		}
		// 处理操作
		s.sqlStrBuilder.WriteString(" " + typ.typ + " ")
		// 处理右节点
		if err := s.buildTable(typ.right); err != nil {
			return err
		}
		// 处理Using
		if len(typ.using) > 0 {
			s.sqlStrBuilder.WriteString("USING(")
			for idx, u := range typ.using {
				if idx > 0 {
					s.sqlStrBuilder.WriteByte(',')
				}
				if err := s.buildColumn(Col(u)); err != nil {
					return err
				}
			}
			s.sqlStrBuilder.WriteByte(')') // using的右括号
		}

		// 处理on部分
		if len(typ.on) > 0 {
			s.sqlStrBuilder.WriteString(" ON ")
			if err := s.buildPredicates(typ.on); err != nil {
				return err
			}
		}

		s.sqlStrBuilder.WriteByte(')') //join的右括号
	default:
		return errs.NewErrUnsupportedTable(table)
	}

	return nil
}
