package slowquery

import (
	"Soil/orm"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockQueryBuilder 返回预设的 Query（或 error），用于在不连接数据库的情况下
// 驱动 slowquery 中间件。
type mockQueryBuilder struct {
	q   *orm.Query
	err error
}

func (m *mockQueryBuilder) Build() (*orm.Query, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.q, nil
}

// TestBuild_ReturnsCallableMiddleware 验证 Build 返回的 Middleware 可被调用，
// 且会调用 next handler。
func TestBuild_ReturnsCallableMiddleware(t *testing.T) {
	mb := NewMiddlewareBuilder(time.Second)
	mw := mb.Build()
	require.NotNil(t, mw)

	called := false
	next := orm.Handler(func(ctx context.Context, queryCtx *orm.QueryContext) *orm.QueryResult {
		called = true
		return &orm.QueryResult{Result: "ok"}
	})

	wrapped := mw(next)
	require.NotNil(t, wrapped)

	qCtx := &orm.QueryContext{
		QueryBuilder: &mockQueryBuilder{q: &orm.Query{SQL: "SELECT 1", Args: []any{}}},
	}
	res := wrapped(context.Background(), qCtx)
	assert.True(t, called, "next handler should be called")
	assert.Equal(t, "ok", res.Result)
}

// TestSlowQuery_LogsWhenExceedingThreshold 验证耗时超过阈值的查询会被记录。
func TestSlowQuery_LogsWhenExceedingThreshold(t *testing.T) {
	var loggedSQL string
	var loggedArgs []any
	var loggedDuration time.Duration
	logged := false

	mb := NewMiddlewareBuilder(20 * time.Millisecond).
		LogFunc(func(query string, args []any, duration time.Duration) {
			logged = true
			loggedSQL = query
			loggedArgs = args
			loggedDuration = duration
		})
	mw := mb.Build()

	next := orm.Handler(func(ctx context.Context, queryCtx *orm.QueryContext) *orm.QueryResult {
		time.Sleep(60 * time.Millisecond) // 模拟慢查询
		return &orm.QueryResult{}
	})

	wrapped := mw(next)
	qCtx := &orm.QueryContext{
		QueryBuilder: &mockQueryBuilder{q: &orm.Query{SQL: "SELECT sleep()", Args: []any{1}}},
	}
	wrapped(context.Background(), qCtx)

	assert.True(t, logged, "slow query should be logged")
	assert.Equal(t, "SELECT sleep()", loggedSQL)
	assert.Equal(t, []any{1}, loggedArgs)
	assert.GreaterOrEqual(t, loggedDuration, 20*time.Millisecond)
}

// TestFastQuery_NotLogged 验证快查询（耗时小于等于阈值）不被记录。
func TestFastQuery_NotLogged(t *testing.T) {
	logged := false
	mb := NewMiddlewareBuilder(time.Second).
		LogFunc(func(query string, args []any, duration time.Duration) {
			logged = true
		})
	mw := mb.Build()

	next := orm.Handler(func(ctx context.Context, queryCtx *orm.QueryContext) *orm.QueryResult {
		return &orm.QueryResult{}
	})

	wrapped := mw(next)
	qCtx := &orm.QueryContext{
		QueryBuilder: &mockQueryBuilder{q: &orm.Query{SQL: "SELECT 1", Args: []any{}}},
	}
	wrapped(context.Background(), qCtx)
	assert.False(t, logged, "fast query should not be logged")
}

// TestThresholdBoundary 验证阈值配置生效：相同 handler 在小阈值下记录、在大阈值下不记录。
func TestThresholdBoundary(t *testing.T) {
	// 大阈值：不记录
	bigLogged := false
	bigMw := NewMiddlewareBuilder(time.Second).
		LogFunc(func(string, []any, time.Duration) { bigLogged = true }).Build()
	bigWrapped := bigMw(func(ctx context.Context, queryCtx *orm.QueryContext) *orm.QueryResult {
		return &orm.QueryResult{}
	})
	bigWrapped(context.Background(), &orm.QueryContext{
		QueryBuilder: &mockQueryBuilder{q: &orm.Query{SQL: "SELECT 1"}},
	})
	assert.False(t, bigLogged, "大阈值不应记录")

	// 小阈值：记录（handler 内 sleep 以确保耗时 > 阈值，规避 Windows 低分辨率时钟）
	smallLogged := false
	smallMw := NewMiddlewareBuilder(time.Nanosecond).
		LogFunc(func(string, []any, time.Duration) { smallLogged = true }).Build()
	smallWrapped := smallMw(func(ctx context.Context, queryCtx *orm.QueryContext) *orm.QueryResult {
		time.Sleep(2 * time.Millisecond)
		return &orm.QueryResult{}
	})
	smallWrapped(context.Background(), &orm.QueryContext{
		QueryBuilder: &mockQueryBuilder{q: &orm.Query{SQL: "SELECT 1"}},
	})
	assert.True(t, smallLogged, "小阈值应记录")
}

// TestBuildQueryBuilderError_NotLogged 验证当 QueryBuilder.Build 返回错误时，
// 中间件不会调用 logFunc（仅当 Build 成功且为慢查询时才记录）。
func TestBuildQueryBuilderError_NotLogged(t *testing.T) {
	logged := false
	mb := NewMiddlewareBuilder(time.Nanosecond).
		LogFunc(func(string, []any, time.Duration) { logged = true })
	mw := mb.Build()

	next := orm.Handler(func(ctx context.Context, queryCtx *orm.QueryContext) *orm.QueryResult {
		return &orm.QueryResult{}
	})

	wrapped := mw(next)
	qCtx := &orm.QueryContext{
		QueryBuilder: &mockQueryBuilder{err: errors.New("build failed")},
	}
	wrapped(context.Background(), qCtx)
	assert.False(t, logged, "Build 出错时不应记录")
}

// TestNewMiddlewareBuilder_DefaultLogFunc 验证 NewMiddlewareBuilder 设置了默认 logFunc
// （未调用 LogFunc 时，慢查询不会 panic，使用 log.Printf）。
func TestNewMiddlewareBuilder_DefaultLogFunc(t *testing.T) {
	mb := NewMiddlewareBuilder(time.Nanosecond)
	mw := mb.Build()

	next := orm.Handler(func(ctx context.Context, queryCtx *orm.QueryContext) *orm.QueryResult {
		return &orm.QueryResult{}
	})

	wrapped := mw(next)
	qCtx := &orm.QueryContext{
		QueryBuilder: &mockQueryBuilder{q: &orm.Query{SQL: "SELECT 1", Args: []any{}}},
	}
	assert.NotPanics(t, func() {
		wrapped(context.Background(), qCtx)
	})
}

// TestLogFunc_ReturnsBuilder 验证 LogFunc 返回 builder 本身以支持链式调用。
func TestLogFunc_ReturnsBuilder(t *testing.T) {
	mb := NewMiddlewareBuilder(time.Second)
	returned := mb.LogFunc(func(string, []any, time.Duration) {})
	assert.Same(t, mb, returned)
}
