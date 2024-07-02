package slowquery

import (
	"Soil/orm"
	"context"
	"log"
	"time"
)

type MiddlewareBuilder struct {
	threshold time.Duration
	logFunc   func(query string, args []any, duration time.Duration)
}

func NewMiddlewareBuilder(threshold time.Duration) *MiddlewareBuilder {
	return &MiddlewareBuilder{
		threshold: threshold,
		logFunc: func(query string, args []any, duration time.Duration) {
			log.Printf("sql: %s, args: %v, duration: %v", query, args, duration)
		},
	}
}

func (m *MiddlewareBuilder) LogFunc(fn func(query string, args []any, duration time.Duration)) *MiddlewareBuilder {
	m.logFunc = fn
	return m
}

func (m *MiddlewareBuilder) Build() orm.Middleware {
	return func(next orm.Handler) orm.Handler {
		return func(ctx context.Context, queryCtx *orm.QueryContext) *orm.QueryResult {
			startTime := time.Now()
			defer func() {
				duration := time.Since(startTime)
				if duration <= m.threshold {
					// 不是慢查询
					return
				}
				// 记录慢查询sql和查询时间
				q, err := queryCtx.QueryBuilder.Build()
				if err == nil {
					m.logFunc(q.SQL, q.Args, duration)
				}
			}()

			return next(ctx, queryCtx)
		}
	}
}
