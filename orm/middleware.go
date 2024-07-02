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

type Handler func(ctx context.Context, queryCtx *QueryContext) *QueryResult
type Middleware func(next Handler) Handler
