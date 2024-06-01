package opentelemetry

import (
	"Soil/web"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

const defaultInstrumentationName = "Soil/web/middleware/opentelemetry"

type MiddlewareBuilder struct {
	Tracer trace.Tracer
}

func (m *MiddlewareBuilder) Build() web.Middleware {
	if m.Tracer == nil {
		m.Tracer = otel.GetTracerProvider().Tracer(defaultInstrumentationName)
	}
}
