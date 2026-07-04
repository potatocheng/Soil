package opentelemetry

import (
	"Soil/orm"
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const instrumentationName = "Soil/orm/middleware/opentelemetry"

type MiddlewareBuilder struct {
	Tracer trace.Tracer
}

func (m *MiddlewareBuilder) Build() orm.Middleware {
	if m.Tracer == nil {
		m.Tracer = otel.GetTracerProvider().Tracer(instrumentationName)
	}

	return func(next orm.Handler) orm.Handler {
		return func(ctx context.Context, queryCtx *orm.QueryContext) *orm.QueryResult {
			_, span := m.Tracer.Start(ctx, "orm:query")
			defer span.End()

			// 记录 SQL 和 args（若可获取）
			if queryCtx != nil && queryCtx.QueryBuilder != nil {
				q, err := queryCtx.QueryBuilder.Build()
				if err == nil && q != nil {
					span.SetAttributes(attribute.String("db.statement", q.SQL))
					if len(q.Args) > 0 {
						span.SetAttributes(attribute.String("db.args", fmt.Sprintf("%v", q.Args)))
					}
				}
			}

			res := next(ctx, queryCtx)

			if res != nil && res.Error != nil {
				span.RecordError(res.Error)
				span.SetStatus(codes.Error, res.Error.Error())
			}

			return res
		}
	}
}
