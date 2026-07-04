package opentelemetry

import (
	"Soil/orm"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// fakeQueryBuilder 实现 orm.QueryBuilder 接口用于测试
type fakeQueryBuilder struct {
	sql  string
	args []any
	err  error
}

func (f *fakeQueryBuilder) Build() (*orm.Query, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &orm.Query{SQL: f.sql, Args: f.args}, nil
}

func findAttr(attrs []attribute.KeyValue, key string) (attribute.KeyValue, bool) {
	for _, kv := range attrs {
		if string(kv.Key) == key {
			return kv, true
		}
	}
	return attribute.KeyValue{}, false
}

func TestOpentelemetry_Build_Success(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer tp.Shutdown(context.Background())

	builder := &MiddlewareBuilder{Tracer: tp.Tracer("test")}
	mw := builder.Build()

	called := false
	var next orm.Handler = func(ctx context.Context, qc *orm.QueryContext) *orm.QueryResult {
		called = true
		return &orm.QueryResult{Result: "ok"}
	}

	handler := mw(next)
	qc := &orm.QueryContext{
		Type:         "INSERT",
		QueryBuilder: &fakeQueryBuilder{sql: "INSERT INTO t VALUES (?)", args: []any{1}},
	}
	res := handler(context.Background(), qc)

	assert.True(t, called, "next handler 应被调用")
	assert.NotNil(t, res)
	assert.Equal(t, "ok", res.Result)

	spans := exporter.GetSpans()
	assert.Len(t, spans, 1, "应创建 1 个 span")
	s := spans[0]
	assert.Equal(t, "orm:query", s.Name)

	sqlAttr, ok := findAttr(s.Attributes, "db.statement")
	assert.True(t, ok, "应记录 db.statement 属性")
	assert.Equal(t, "INSERT INTO t VALUES (?)", sqlAttr.Value.AsString())

	argsAttr, ok := findAttr(s.Attributes, "db.args")
	assert.True(t, ok, "应记录 db.args 属性")
	assert.NotEmpty(t, argsAttr.Value.AsString())

	assert.Equal(t, codes.Unset, s.Status.Code, "成功时不应设置 Error 状态")
}

func TestOpentelemetry_Build_Error(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer tp.Shutdown(context.Background())

	builder := &MiddlewareBuilder{Tracer: tp.Tracer("test")}
	mw := builder.Build()

	errExpected := errors.New("db error")
	var next orm.Handler = func(ctx context.Context, qc *orm.QueryContext) *orm.QueryResult {
		return &orm.QueryResult{Error: errExpected}
	}

	handler := mw(next)
	qc := &orm.QueryContext{
		Type:         "INSERT",
		QueryBuilder: &fakeQueryBuilder{sql: "INSERT INTO t VALUES (?)"},
	}
	res := handler(context.Background(), qc)

	assert.NotNil(t, res)
	assert.Equal(t, errExpected, res.Error)

	spans := exporter.GetSpans()
	assert.Len(t, spans, 1)
	s := spans[0]
	assert.Equal(t, codes.Error, s.Status.Code, "应设置 Error 状态")
	assert.Equal(t, "db error", s.Status.Description)
	assert.NotEmpty(t, s.Events, "应通过 RecordError 记录 error 事件")
}

func TestOpentelemetry_Build_NoArgs(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer tp.Shutdown(context.Background())

	builder := &MiddlewareBuilder{Tracer: tp.Tracer("test")}
	mw := builder.Build()

	var next orm.Handler = func(ctx context.Context, qc *orm.QueryContext) *orm.QueryResult {
		return &orm.QueryResult{Result: "ok"}
	}

	handler := mw(next)
	qc := &orm.QueryContext{
		Type:         "SELECT",
		QueryBuilder: &fakeQueryBuilder{sql: "SELECT 1"},
	}
	res := handler(context.Background(), qc)
	assert.NotNil(t, res)

	spans := exporter.GetSpans()
	assert.Len(t, spans, 1)
	s := spans[0]

	sqlAttr, ok := findAttr(s.Attributes, "db.statement")
	assert.True(t, ok)
	assert.Equal(t, "SELECT 1", sqlAttr.Value.AsString())

	_, hasArgs := findAttr(s.Attributes, "db.args")
	assert.False(t, hasArgs, "无参数时不应记录 db.args")
}

func TestOpentelemetry_Build_NilTracer(t *testing.T) {
	// 验证 Tracer 为 nil 时使用默认 provider，Build 不应 panic
	builder := &MiddlewareBuilder{}
	mw := builder.Build()
	assert.NotNil(t, mw)
}
