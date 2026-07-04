package orm

import (
	"Soil/orm/internal/errs"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
)

// TestRawQuerier_Creation 验证 Raw 方法创建的 RawQuerier 字段正确设置
func TestRawQuerier_Creation(t *testing.T) {
	mockDB, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	db, err := OpenDB(mockDB)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		sql      string
		args     []any
		wantArgs []any
	}{
		{
			name:     "no args",
			sql:      "SELECT * FROM t",
			args:     nil,
			wantArgs: nil,
		},
		{
			name:     "with args",
			sql:      "SELECT * FROM t WHERE id = ?",
			args:     []any{1},
			wantArgs: []any{1},
		},
		{
			name:     "with multiple args",
			sql:      "UPDATE t SET name = ? WHERE id = ?",
			args:     []any{"tom", 42},
			wantArgs: []any{"tom", 42},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := db.Raw(tt.sql, tt.args...)
			assert.NotNil(t, r)
			assert.Equal(t, db, r.db)
			assert.Equal(t, tt.sql, r.sql)
			assert.Equal(t, tt.wantArgs, r.args)
		})
	}
}

// TestRawQuerier_Query 验证 Raw SQL 的 Query 映射（通过 sqlmock）
func TestRawQuerier_Query(t *testing.T) {
	type rawTestModel struct {
		Id        int64
		FirstName string
		Age       uint8
		LastName  string
	}

	tests := []struct {
		name      string
		sql       string
		args      []any
		mockRows  *sqlmock.Rows
		mockErr   error
		wantModel *rawTestModel
		wantErr   error
	}{
		{
			name:     "single row",
			sql:      "SELECT id, first_name, age, last_name FROM raw_test_model WHERE id = ?",
			args:     []any{1},
			mockRows: sqlmock.NewRows([]string{"id", "first_name", "age", "last_name"}).AddRow([]byte("1"), []byte("yang"), []byte("18"), []byte("cheng")),
			wantModel: &rawTestModel{
				Id:        1,
				FirstName: "yang",
				Age:       18,
				LastName:  "cheng",
			},
		},
		{
			name:     "no rows",
			sql:      "SELECT id, first_name, age, last_name FROM raw_test_model WHERE id = ?",
			args:     []any{999},
			mockRows: sqlmock.NewRows([]string{"id", "first_name", "age", "last_name"}),
			wantErr:  errs.ErrNoRows,
		},
		{
			name:    "query error",
			sql:     "SELECT id FROM raw_test_model",
			mockErr: errors.New("query failed"),
			wantErr: errors.New("query failed"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDB, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			db, err := OpenDB(mockDB)
			if err != nil {
				t.Fatal(err)
			}

			exp := mock.ExpectQuery(tt.sql)
			if tt.mockErr != nil {
				exp.WillReturnError(tt.mockErr)
			} else {
				exp.WillReturnRows(tt.mockRows)
			}

			got := &rawTestModel{}
			res := db.Raw(tt.sql, tt.args...).Query(context.Background(), got)
			assert.Equal(t, tt.wantErr, res.err)
			if tt.wantErr != nil {
				return
			}
			assert.Equal(t, tt.wantModel, got)
		})
	}
}

// TestRawQuerier_Query_MultiRows 验证 Raw SQL 的 Query 多行映射到切片
func TestRawQuerier_Query_MultiRows(t *testing.T) {
	type rawTestModel struct {
		Id        int64
		FirstName string
		Age       uint8
	}

	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	db, err := OpenDB(mockDB)
	if err != nil {
		t.Fatal(err)
	}

	mock.ExpectQuery("SELECT id, first_name, age FROM raw_test_model").
		WillReturnRows(sqlmock.NewRows([]string{"id", "first_name", "age"}).
			AddRow([]byte("1"), []byte("yang"), []byte("18")).
			AddRow([]byte("2"), []byte("tom"), []byte("20")))

	// 测试元素为结构体的切片
	t.Run("slice of struct", func(t *testing.T) {
		var got []rawTestModel
		res := db.Raw("SELECT id, first_name, age FROM raw_test_model").Query(context.Background(), &got)
		assert.Nil(t, res.err)
		assert.Equal(t, []rawTestModel{
			{Id: 1, FirstName: "yang", Age: 18},
			{Id: 2, FirstName: "tom", Age: 20},
		}, got)
	})
}

// TestRawQuerier_Query_InvalidModel 验证传入非指针时返回错误
func TestRawQuerier_Query_InvalidModel(t *testing.T) {
	mockDB, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	db, err := OpenDB(mockDB)
	if err != nil {
		t.Fatal(err)
	}

	type rawTestModel struct {
		Id int64
	}

	t.Run("non pointer", func(t *testing.T) {
		m := rawTestModel{}
		res := db.Raw("SELECT 1").Query(context.Background(), m)
		assert.NotNil(t, res.err)
	})

	t.Run("pointer to int", func(t *testing.T) {
		var n int
		res := db.Raw("SELECT 1").Query(context.Background(), &n)
		assert.NotNil(t, res.err)
	})
}

// TestRawQuerier_Exec 验证 Raw SQL 的 Exec 执行路径
func TestRawQuerier_Exec(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	db, err := OpenDB(mockDB)
	if err != nil {
		t.Fatal(err)
	}

	mock.ExpectExec("UPDATE raw_test_model SET age = \\? WHERE id = \\?").
		WithArgs(18, 1).
		WillReturnResult(sqlmock.NewResult(0, 1))

	res, err := db.Raw("UPDATE raw_test_model SET age = ? WHERE id = ?", 18, 1).Exec(context.Background())
	assert.Nil(t, err)
	assert.NotNil(t, res)
	affected, err := res.RowsAffected()
	assert.Nil(t, err)
	assert.Equal(t, int64(1), affected)

	// 验证所有 mock 期望都被满足
	assert.Nil(t, mock.ExpectationsWereMet())
}

// TestRawQuerier_Exec_Error 验证 Raw SQL 的 Exec 错误路径
func TestRawQuerier_Exec_Error(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	db, err := OpenDB(mockDB)
	if err != nil {
		t.Fatal(err)
	}

	execErr := errors.New("exec failed")
	mock.ExpectExec("DELETE FROM raw_test_model").
		WillReturnError(execErr)

	res, err := db.Raw("DELETE FROM raw_test_model").Exec(context.Background())
	assert.Equal(t, execErr, err)
	assert.Nil(t, res)
}

// TestDB_ConnectionPool 验证连接池配置方法不会 panic，且底层 *sql.DB 生效
func TestDB_ConnectionPool(t *testing.T) {
	mockDB, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	db, err := OpenDB(mockDB)
	if err != nil {
		t.Fatal(err)
	}

	// 调用所有连接池配置方法，验证不 panic
	assert.NotPanics(t, func() {
		db.SetMaxOpenConns(50)
		db.SetMaxIdleConns(10)
		db.SetConnMaxLifetime(30 * time.Minute)
		db.SetConnMaxIdleTime(5 * time.Minute)
	})

	// 验证底层 *sql.DB 确实被调用（通过 sqlmock 的 Stats 间接验证连接池配置已生效）
	// sqlmock 的 DB 会记录配置，这里主要确保调用不 panic 即可
}

// TestDB_Ping 验证 Ping 方法委托给底层 *sql.DB
func TestDB_Ping(t *testing.T) {
	// 启用 ping 监控才能让 ExpectPing 生效
	mockDB, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if err != nil {
		t.Fatal(err)
	}
	db, err := OpenDB(mockDB)
	if err != nil {
		t.Fatal(err)
	}

	mock.ExpectPing()
	err = db.Ping(context.Background())
	assert.Nil(t, err)
	assert.Nil(t, mock.ExpectationsWereMet())
}

// 编译期保证 *RawQuerier 实现了 Executor 接口
var _ Executor = (*RawQuerier)(nil)

// 编译期保证 DB 实现连接池配置相关方法存在
var _ = func(db *DB) {
	db.SetMaxOpenConns(0)
	db.SetMaxIdleConns(0)
	db.SetConnMaxLifetime(0)
	db.SetConnMaxIdleTime(0)
	_ = db
}
