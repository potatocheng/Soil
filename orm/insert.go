package orm

import (
	"Soil/orm/internal/errs"
	"Soil/orm/internal/model"
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"time"
)

type UpsertBuilder[T any] struct {
	inserter        *Inserter[T]
	conflictColumns []string
}

// setTimestampField 通过反射将 val（指向结构体的指针）上 field 指定的字段设置为时间戳 now。
// 支持 time.Time 与 *time.Time 两种常见字段类型；其它类型在可赋值时直接赋值，否则返回错误。
// 该函数供 Inserter/Updater 在 Exec 阶段自动填充 CreatedAt/UpdatedAt 使用。
func setTimestampField(val any, field *model.Field, now time.Time) error {
	v := reflect.ValueOf(val).Elem()
	fv := v.FieldByName(field.GoName)
	if !fv.IsValid() {
		return errs.NewErrUnknownField(field.GoName)
	}
	if !fv.CanSet() {
		return fmt.Errorf("orm: 字段 %s 不可设置", field.GoName)
	}
	switch fv.Type() {
	case reflect.TypeOf(time.Time{}):
		fv.Set(reflect.ValueOf(now))
	case reflect.TypeOf((*time.Time)(nil)):
		fv.Set(reflect.ValueOf(&now))
	default:
		if reflect.ValueOf(now).Type().AssignableTo(fv.Type()) {
			fv.Set(reflect.ValueOf(now))
		} else {
			return fmt.Errorf("orm: 字段 %s 类型 %s 不支持设置时间戳", field.GoName, fv.Type())
		}
	}
	return nil
}

// isFieldZero 通过反射判断 val 上 field 指定的字段是否为零值。
// 用于自动填充时间戳时跳过用户/钩子已设置的值（仅 CreatedAt 使用）。
func isFieldZero(val any, field *model.Field) bool {
	v := reflect.ValueOf(val).Elem()
	fv := v.FieldByName(field.GoName)
	if !fv.IsValid() {
		return true
	}
	return fv.IsZero()
}

type Upsert struct {
	conflictColumns []string
	assigns         []Assignable
}

func (s *UpsertBuilder[T]) ConflictColumns(cols ...string) *UpsertBuilder[T] {
	s.conflictColumns = cols
	return s
}

// Update 调用了这个函数后就表示Upsert的数据设置完成，之后调用Insert的函数继续设置Insert的数据
func (s *UpsertBuilder[T]) Update(assigns ...Assignable) *Inserter[T] {
	s.inserter.upsert = &Upsert{
		conflictColumns: s.conflictColumns,
		assigns:         assigns,
	}

	return s.inserter
}

type Inserter[T any] struct {
	builder
	values    []*T
	columns   []string
	session   Session
	chunkSize int // 分块大小，0 表示不分块（一次插完）

	upsert *Upsert
}

func NewInserter[T any](session Session) *Inserter[T] {
	c := session.getCore()
	return &Inserter[T]{
		builder: builder{
			core:   c,
			quoter: c.dialect.quoter(),
		},
		session: session,
	}
}

func (i *Inserter[T]) OnDuplicateKey() *UpsertBuilder[T] {
	return &UpsertBuilder[T]{
		inserter: i,
	}
}

func (i *Inserter[T]) Values(val ...*T) *Inserter[T] {
	i.values = append(i.values, val...)

	return i
}

func (i *Inserter[T]) Columns(col ...string) *Inserter[T] {
	i.columns = append(i.columns, col...)

	return i
}

// ChunkSize 设置批量插入的分块大小。n<=0 表示不分块（一次性插入所有 values）；
// n>0 且 values 数量超过 n 时，Exec 会将 values 按 n 分批，每批单独 Build+执行。
// 返回 Inserter 以便链式调用。
func (i *Inserter[T]) ChunkSize(n int) *Inserter[T] {
	i.chunkSize = n
	return i
}

func (i *Inserter[T]) Build() (*Query, error) {
	if len(i.values) == 0 {
		return nil, errs.ErrInsertZeroRow
	}

	var err error
	// 获得元数据
	i.model, err = i.r.Get(new(T))
	if err != nil {
		return nil, err
	}

	i.sqlStrBuilder.WriteString("INSERT INTO ")
	i.quote(i.model.TableName)
	i.sqlStrBuilder.WriteByte('(')

	// 获取列名
	fields := i.model.Fields
	if len(i.columns) != 0 {
		//用户指定列名
		fields = make([]*model.Field, 0, len(i.columns))
		for _, col := range i.columns {
			field, ok := i.model.FieldMap[col]
			if !ok {
				return nil, errs.NewErrUnknownField(col)
			}

			fields = append(fields, field)
		}
	}

	//for k, v := range meta.FieldMap //这样不行，因为每次遍历k-v对顺序不同
	for idx, field := range fields {
		if idx != 0 {
			i.sqlStrBuilder.WriteByte(',')
		}
		i.quote(field.ColName)
	}

	i.sqlStrBuilder.WriteByte(')')

	// 处理VALUES部分,处理参数
	i.sqlStrBuilder.WriteString(" VALUES ")
	i.args = make([]any, 0, len(fields)*len(i.values)+1)
	for j, val := range i.values {
		if j != 0 {
			i.sqlStrBuilder.WriteByte(',')
		}
		valDealer := i.valCreator(val, i.model)
		i.sqlStrBuilder.WriteByte('(')
		for idx, field := range fields {
			if idx != 0 {
				i.sqlStrBuilder.WriteByte(',')
			}
			i.sqlStrBuilder.WriteByte('?')
			fdVal, err := valDealer.GetFieldValue(field.GoName)
			if err != nil {
				return nil, err
			}
			i.args = append(i.args, fdVal)
		}
		i.sqlStrBuilder.WriteByte(')')
	}

	// 处理Upsert部分
	if i.upsert != nil {
		err = i.dialect.buildUpsert(&(i.builder), i.upsert)
		if err != nil {
			return nil, err
		}
	}

	i.sqlStrBuilder.WriteByte(';')
	return &Query{
		SQL:  i.sqlStrBuilder.String(),
		Args: i.args,
	}, nil
}

//	func (i *Inserter[T]) buildAssigment(a Assignable) error {
//		switch assign := a.(type) {
//		case Assignment:
//			i.sqlStrBuilder.WriteByte('`')
//			field, ok := i.model.FieldMap[assign.column]
//			if !ok {
//				return errs.NewErrUnknownField(assign.column)
//			}
//			i.sqlStrBuilder.WriteString(field.ColName)
//			i.sqlStrBuilder.WriteByte('`')
//			i.sqlStrBuilder.WriteString("=?")
//			i.args = append(i.args, assign.val)
//		case Column:
//			field, ok := i.model.FieldMap[assign.name]
//			if !ok {
//				return errs.NewErrUnknownField(assign.name)
//			}
//			i.sqlStrBuilder.WriteByte('`')
//			i.sqlStrBuilder.WriteString(field.ColName)
//			i.sqlStrBuilder.WriteString("`=VALUES(`")
//			i.sqlStrBuilder.WriteString(field.ColName)
//			i.sqlStrBuilder.WriteString("`)")
//		}
//
//		return nil
//	}
func (i *Inserter[T]) Exec(ctx context.Context) Result {
	var err error
	i.model, err = i.r.Get(new(T))
	if err != nil {
		return Result{err: err}
	}

	// BeforeInsert 钩子：在 Build/Exec 之前调用，钩子内对模型的修改会反映到生成的 SQL。
	// values 可能是批量插入，需对每个元素调用。
	for _, v := range i.values {
		if h, ok := any(v).(BeforeInsert); ok {
			if e := h.BeforeInsert(ctx); e != nil {
				return Result{err: e}
			}
		}
	}

	// 自动填充 CreatedAt：在钩子之后、Build 之前执行，确保生成 SQL 时字段已有值。
	// 仅在字段为零值时填充，以尊重钩子或用户显式设置的值。
	if i.model.CreatedAtField != nil {
		now := time.Now()
		for _, v := range i.values {
			if isFieldZero(v, i.model.CreatedAtField) {
				if e := setTimestampField(v, i.model.CreatedAtField, now); e != nil {
					return Result{err: e}
				}
			}
		}
	}

	// 执行：分块或一次性。钩子已在上面统一调用一次，下面不再触发。
	var res *QueryResult
	if i.chunkSize > 0 && len(i.values) > i.chunkSize {
		res = i.execChunked(ctx)
	} else {
		res = exec(ctx, i.core, i.session, &QueryContext{
			Type:         "INSERT",
			QueryBuilder: i,
			Model:        i.model,
		})
	}

	var sqlRes sql.Result
	if res.Result != nil {
		sqlRes = res.Result.(sql.Result)
	}

	// AfterInsert 钩子：仅在 SQL 执行成功后调用。
	if res.Error == nil {
		for _, v := range i.values {
			if h, ok := any(v).(AfterInsert); ok {
				if e := h.AfterInsert(ctx); e != nil {
					return Result{err: e}
				}
			}
		}
	}

	return Result{
		err: res.Error,
		res: sqlRes,
	}
}

// execChunked 将 values 按 chunkSize 分块，分别 Build + 执行，汇总影响行数。
// 钩子（BeforeInsert/AfterInsert）已在 Exec 中对全部 values 调用一次，这里不再触发。
// 某批失败则停止后续批次并返回该错误。
func (i *Inserter[T]) execChunked(ctx context.Context) *QueryResult {
	chunks := splitChunk(i.values, i.chunkSize)

	// 临时替换 values 用于 Build，结束后恢复以便 Inserter 可被复用。
	origValues := i.values
	defer func() { i.values = origValues }()

	var totalRows int64
	var lastID int64
	for _, chunk := range chunks {
		i.values = chunk
		// Build 会在已有 sqlStrBuilder 上追加，需先 Reset。
		i.sqlStrBuilder.Reset()
		q, err := i.Build()
		if err != nil {
			return &QueryResult{Error: err, Result: Result{err: err}}
		}
		// 用包装好的 Query 透传给 exec，避免再次 Build。
		res := exec(ctx, i.core, i.session, &QueryContext{
			Type:         "INSERT",
			QueryBuilder: &chunkQueryBuilder{query: q},
			Model:        i.model,
		})
		if res.Error != nil {
			return res
		}
		if sqlRes, ok := res.Result.(sql.Result); ok && sqlRes != nil {
			if n, e := sqlRes.RowsAffected(); e == nil {
				totalRows += n
			}
			if id, e := sqlRes.LastInsertId(); e == nil {
				lastID = id
			}
		}
	}

	agg := &aggregatedResult{rowsAffected: totalRows, lastInsertID: lastID}
	return &QueryResult{Result: Result{res: agg}, Error: nil}
}

// chunkQueryBuilder 包装一个已构建好的 Query，使其满足 QueryBuilder 接口。
// 分块插入时用于将每个 chunk 的 Query 透传给 exec，避免重复 Build。
type chunkQueryBuilder struct {
	query *Query
}

func (c *chunkQueryBuilder) Build() (*Query, error) {
	return c.query, nil
}

// aggregatedResult 汇总多批 INSERT 的执行结果，实现 sql.Result 接口。
// rowsAffected 为各批影响行数之和；lastInsertID 取最后一批的 LAST_INSERT_ID。
type aggregatedResult struct {
	rowsAffected int64
	lastInsertID int64
}

func (a *aggregatedResult) LastInsertId() (int64, error) {
	return a.lastInsertID, nil
}

func (a *aggregatedResult) RowsAffected() (int64, error) {
	return a.rowsAffected, nil
}

// splitChunk 将切片按 size 分块。size<=0 或输入为空时返回包含原切片的单个分块。
func splitChunk[T any](values []T, size int) [][]T {
	if size <= 0 || len(values) == 0 {
		return [][]T{values}
	}
	var chunks [][]T
	for i := 0; i < len(values); i += size {
		end := i + size
		if end > len(values) {
			end = len(values)
		}
		chunks = append(chunks, values[i:end])
	}
	return chunks
}
