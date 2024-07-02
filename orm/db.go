package orm

import (
	"Soil/orm/internal/errs"
	"Soil/orm/internal/model"
	"Soil/orm/internal/valuer"
	"context"
	"database/sql"
)

type DBOption func(*DB)

type DB struct {
	db *sql.DB
	core
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
		core: core{
			r:          model.NewRegistry(),
			valCreator: valuer.NewUnsafeValue,
			dialect:    MySQL,
		},
		db: db,
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

func (db *DB) getCore() core {
	return db.core
}

func (db *DB) queryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return db.db.QueryContext(ctx, query, args...)
}

func (db *DB) execContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return db.db.ExecContext(ctx, query, args...)
}

func (db *DB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*Tx, error) {
	tx, err := db.db.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &Tx{
		tx: tx,
		db: db,
	}, nil
}

func (db *DB) DoTx(ctx context.Context,
	fn func(ctx context.Context, tx *Tx) error,
	opts *sql.TxOptions) (err error) {
	var tx *Tx
	tx, err = db.BeginTx(ctx, opts)
	if err != nil {
		return err
	}

	panicked := true
	defer func() {
		if panicked || err != nil {
			e := tx.Rollback()
			if e != nil {
				err = errs.NewErrFailToRollbackTx(e, err, panicked)
			}
		} else {
			err = tx.Commit()
		}
	}()

	err = fn(ctx, tx)
	panicked = false

	return err
}

func (db *DB) Close() error {
	return db.db.Close()
}

func DBWithMiddlewares(mdls ...Middleware) DBOption {
	return func(db *DB) {
		db.middlewares = mdls
	}
}
