package orm

import (
	"Soil/orm/internal/model"
	"Soil/orm/internal/valuer"
	"database/sql"
)

type DBOption func(*DB)

type DB struct {
	r          model.Registry
	db         *sql.DB
	valCreator valuer.Creator //指定结果处理方法
	dialect    Dialect
}

func Open(driverName string, dataSourceName string, opts ...DBOption) (*DB, error) {
	db, err := sql.Open(driverName, dataSourceName)
	if err != nil {
		return nil, err
	}

	return OpenDB(db, opts...)
}

// OpenDB 默认使用的Dialect时MySQL，默认使用的结果集是通过Unsafe(Reflect速度慢)
func OpenDB(db *sql.DB, opts ...DBOption) (*DB, error) {
	res := &DB{
		r:          model.NewRegistry(),
		db:         db,
		valCreator: valuer.NewUnsafeValue,
		dialect:    MySQL,
	}

	for _, opt := range opts {
		opt(res)
	}

	return res, nil
}

func DBUseReflect() DBOption {
	return func(db *DB) {
		db.valCreator = valuer.NewReflectValue
	}
}

func DBWithDialect(dialect Dialect) DBOption {
	return func(db *DB) {
		db.dialect = dialect
	}
}
