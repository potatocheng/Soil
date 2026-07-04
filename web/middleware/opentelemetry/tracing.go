package opentelemetry

import (
	"Soil/web"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
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

	return func(next web.HandleFunc) web.HandleFunc {
		return func(ctx *web.Context) {
			req := ctx.Req
			opts := []trace.SpanStartOption{
				trace.WithAttributes(
					attribute.String("http.method", req.Method),
					attribute.String("http.url", req.URL.Path),
					attribute.String("http.host", req.Host),
					attribute.String("http.scheme", req.URL.Scheme),
				),
			}

			spanCtx, span := m.Tracer.Start(req.Context(), "http.request", opts...)
			defer span.End()

			req = req.WithContext(spanCtx)
			ctx.Req = req

			next(ctx)

			statusCode := ctx.RespStatusCode
			if statusCode == 0 {
				statusCode = http.StatusOK
			}
			span.SetAttributes(attribute.Int("http.status_code", statusCode))

			if statusCode >= 400 {
				span.RecordError(&HTTPError{StatusCode: statusCode})
			}
		}
	}
}

type HTTPError struct {
	StatusCode int
}

func (e *HTTPError) Error() string {
	return http.StatusText(e.StatusCode)
}
