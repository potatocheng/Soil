package valuer

import (
	"Soil/orm/internal/model"
	"database/sql"
)

// Valuer 处理结果集方法的抽象，比如可以通过unsafe或者reflect来处理
type Valuer interface {
	SetColumns(rows *sql.Rows) error
}

type Creator func(val any, meta *model.Model) Valuer
