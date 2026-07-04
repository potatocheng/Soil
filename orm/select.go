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

	// 处理where之后的条件（含软删除过滤）
	if err = s.buildWhereWithSoftDelete(s.where); err != nil {
		return nil, err
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
	} else if s.offset > 0 {
		// MySQL 不允许 OFFSET 不带 LIMIT，这里补充一个极大值 LIMIT 表示无限制
		s.sqlStrBuilder.WriteString(" LIMIT 18446744073709551615")
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
	// 提前创建结果实例，便于在其上调用 BeforeQuery 钩子。
	retVal := new(T)

	// BeforeQuery 钩子：在 Build/Exec 之前调用。
	if h, ok := any(retVal).(BeforeQuery); ok {
		if e := h.BeforeQuery(ctx); e != nil {
			return &QueryResult{Error: e}
		}
	}

	query, err := qc.QueryBuilder.Build()
	if err != nil {
		return &QueryResult{Error: err}
	}

	//执行sql查询语句,
	rows, err := session.queryContext(ctx, query.SQL, query.Args...)
	if err != nil {
		return &QueryResult{Error: err}
	}
	defer rows.Close()

	if !rows.Next() {
		return &QueryResult{Error: errs.ErrNoRows}
	}

	valuer := c.valCreator(retVal, c.model)
	err = valuer.SetColumns(rows)
	if err != nil {
		return &QueryResult{Error: err}
	}

	// Get 期望返回单行，若还存在下一行则视为返回了过多数据
	if rows.Next() {
		return &QueryResult{Error: errs.ErrTooManyRows}
	}

	// AfterQuery 钩子：仅在结果填充成功后调用。
	if h, ok := any(retVal).(AfterQuery); ok {
		if e := h.AfterQuery(ctx); e != nil {
			return &QueryResult{Error: e}
		}
	}

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

// Paginate 是分页助手：page 从 1 开始（page=1 表示第一页，OFFSET 0），
// size 为每页大小。page<1 时按 1 处理；size<=0 时按 10 处理。
// 内部等价于调用 Limit(size) 和 Offset((page-1)*size)，返回 Selector 以便链式调用。
func (s *Selector[T]) Paginate(page, size int) *Selector[T] {
	if page < 1 {
		page = 1
	}
	if size <= 0 {
		size = 10
	}
	s.limit = size
	s.offset = (page - 1) * size
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
			s.sqlStrBuilder.WriteString(" USING (")
			for idx, u := range typ.using {
				if idx > 0 {
					s.sqlStrBuilder.WriteString(", ")
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
