package orm

import "database/sql"

type DBOption func(*DB)

type DB struct {
	r  *registry
	db *sql.DB
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
		r:  newRegistry(),
		db: db,
	}

	for _, opt := range opts {
		opt(res)
	}

	return res, nil
}
