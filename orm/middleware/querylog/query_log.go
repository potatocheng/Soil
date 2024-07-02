package querylog

import (
	"Soil/orm"
	"context"
	"log"
)

type MiddlewareBuilder struct {
	logFunc func(sql string, args []any)
}

func (m *MiddlewareBuilder) WithLogFunc(logFunc func(sql string, args []any)) *MiddlewareBuilder {
	m.logFunc = logFunc
	return m
}

func NewMiddlewareBuilder() *MiddlewareBuilder {
	return &MiddlewareBuilder{
		logFunc: func(sql string, args []any) {
			log.Printf("sql: %s, args: %v", sql, args)
		},
	}
}

func (m *MiddlewareBuilder) Build() orm.Middleware {
	return func(next orm.Handler) orm.Handler {
		return func(ctx context.Context, queryCtx *orm.QueryContext) *orm.QueryResult {
			query, err := queryCtx.QueryBuilder.Build()
			if err != nil {
				return &orm.QueryResult{Error: err}
			}
			m.logFunc(query.SQL, query.Args)
			res := next(ctx, queryCtx)
			return res
		}
	}
}
