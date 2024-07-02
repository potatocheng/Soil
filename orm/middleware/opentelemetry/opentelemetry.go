package opentelemetry

import (
	"go.opentelemetry.io/otel/trace"
)

const instrumentationName = "Soil/orm/middleware/opentelemetry"

type MiddlewareBuilder struct {
	Tracer trace.Tracer
}

//func (m MiddlewareBuilder) Build() orm.Middleware {
//	return func(next orm.Handler) orm.Handler {
//		return func(ctx context.Context, queryCtx *orm.QueryContext) *orm.QueryResult {
//
//		}
//	}
//}
