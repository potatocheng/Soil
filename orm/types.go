package orm

import (
	"context"
	"database/sql"
)

// Querier 处理SELECT语句的最终结果， 这里的T表示要查询哪个表
type Querier[T any] interface {
	Get(ctx context.Context) (*T, error)
	GetMulti(ctx context.Context) (*T, error)
}

// Executor 处理INSERT, DELETE和UPDATE的最终结果
type Executor interface {
	Exec(ctx context.Context) (sql.Result, error)
}

type Query struct {
	SQL  string
	Args []any
}

type QueryBuilder interface {
	Build() (*Query, error)
}
