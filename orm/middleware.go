package orm

import (
	"Soil/orm/internal/model"
	"context"
)

type QueryContext struct {
	Type string

	QueryBuilder QueryBuilder
	Model        *model.Model
}

type QueryResult struct {
	Result any
	Error  error
}

// Err 返回查询过程中发生的错误（若有）。
// 调用方可使用 errors.Is(err, errs.ErrXxx) 对 sentinel 错误进行匹配。
func (q QueryResult) Err() error {
	return q.Error
}

type Handler func(ctx context.Context, queryCtx *QueryContext) *QueryResult
type Middleware func(next Handler) Handler
