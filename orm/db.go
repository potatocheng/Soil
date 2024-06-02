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
	valCreator valuer.Creator //
}

func Open(driverName string, dataSourceName string, opts ...DBOption) (*DB, error) {
	db, err := sql.Open(driverName, dataSourceName)
	if err != nil {
		return nil, err
	}

	return OpenDB(db, opts...)
}

func OpenDB(db *sql.DB, opts ...DBOption) (*DB, error) {
	res := &DB{
		r:          model.NewRegistry(),
		db:         db,
		valCreator: valuer.NewUnsafeValue,
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
