package orm

import (
	"Soil/orm/internal/errs"
	"Soil/orm/internal/model"
	"Soil/orm/internal/valuer"
	"context"
	"database/sql"
	"errors"
	"reflect"
	"time"
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
				err = errs.NewErrFailToRollbackTx(err, e, panicked)
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

// SetMaxOpenConns 设置数据库的最大打开连接数，委托给底层 *sql.DB
func (db *DB) SetMaxOpenConns(n int) { db.db.SetMaxOpenConns(n) }

// SetMaxIdleConns 设置数据库的最大空闲连接数，委托给底层 *sql.DB
func (db *DB) SetMaxIdleConns(n int) { db.db.SetMaxIdleConns(n) }

// SetConnMaxLifetime 设置连接的最大存活时间，委托给底层 *sql.DB
func (db *DB) SetConnMaxLifetime(d time.Duration) { db.db.SetConnMaxLifetime(d) }

// SetConnMaxIdleTime 设置连接的最大空闲时间，委托给底层 *sql.DB
func (db *DB) SetConnMaxIdleTime(d time.Duration) { db.db.SetConnMaxIdleTime(d) }

// Ping 检查与数据库的连接是否仍然有效，委托给底层 *sql.DB
func (db *DB) Ping(ctx context.Context) error { return db.db.PingContext(ctx) }

// Raw 创建原生 SQL 查询/执行器，用于直接执行 SQL 而不经过 ORM 的 SQL 构建过程
func (db *DB) Raw(sql string, args ...any) *RawQuerier {
	return &RawQuerier{db: db, sql: sql, args: args}
}

// RawQuerier 原生 SQL 查询/执行器，支持直接执行 SQL 语句
type RawQuerier struct {
	db   *DB
	sql  string
	args []any
}

// Query 执行原生 SELECT 语句，将结果映射到 model
// model 为指向结构体的指针时映射单行（无数据返回 errs.ErrNoRows）；
// model 为指向切片的指针时映射多行（切片元素可为结构体或结构体指针）。
// 由于 Raw SQL 绕过 SQL 构建过程，valuer 仍通过模型注册表获取列与字段的映射关系。
func (r *RawQuerier) Query(ctx context.Context, model any) Result {
	rv := reflect.ValueOf(model)
	if rv.Kind() != reflect.Ptr {
		return Result{err: errors.New("orm: Raw Query 的 model 必须是指针")}
	}
	elem := rv.Elem()

	// 根据目标类型（结构体或切片）确定用于获取元数据的实例
	var metaVal any
	switch elem.Kind() {
	case reflect.Struct:
		metaVal = model
	case reflect.Slice:
		elemType := elem.Type().Elem()
		if elemType.Kind() == reflect.Ptr {
			elemType = elemType.Elem()
		}
		if elemType.Kind() != reflect.Struct {
			return Result{err: errors.New("orm: Raw Query 的切片元素必须是结构体或结构体指针")}
		}
		metaVal = reflect.New(elemType).Interface()
	default:
		return Result{err: errors.New("orm: Raw Query 的 model 必须指向结构体或切片")}
	}

	m, err := r.db.r.Get(metaVal)
	if err != nil {
		return Result{err: err}
	}

	rows, err := r.db.queryContext(ctx, r.sql, r.args...)
	if err != nil {
		return Result{err: err}
	}
	defer rows.Close()

	switch elem.Kind() {
	case reflect.Struct:
		// 单行映射
		if !rows.Next() {
			if err = rows.Err(); err != nil {
				return Result{err: err}
			}
			return Result{err: errs.ErrNoRows}
		}
		v := r.db.valCreator(model, m)
		if err = v.SetColumns(rows); err != nil {
			return Result{err: err}
		}
		if err = rows.Err(); err != nil {
			return Result{err: err}
		}
		return Result{}
	case reflect.Slice:
		// 多行映射
		elemType := elem.Type().Elem()
		isPtr := elemType.Kind() == reflect.Ptr
		structType := elemType
		if isPtr {
			structType = elemType.Elem()
		}
		for rows.Next() {
			newVal := reflect.New(structType)
			v := r.db.valCreator(newVal.Interface(), m)
			if err = v.SetColumns(rows); err != nil {
				return Result{err: err}
			}
			if isPtr {
				elem.Set(reflect.Append(elem, newVal))
			} else {
				elem.Set(reflect.Append(elem, newVal.Elem()))
			}
		}
		if err = rows.Err(); err != nil {
			return Result{err: err}
		}
		return Result{}
	}
	return Result{}
}

// Exec 执行原生非查询语句（INSERT/UPDATE/DELETE 等），返回 sql.Result
func (r *RawQuerier) Exec(ctx context.Context) (sql.Result, error) {
	return r.db.execContext(ctx, r.sql, r.args...)
}

func DBWithMiddlewares(mdls ...Middleware) DBOption {
	return func(db *DB) {
		db.middlewares = mdls
	}
}
