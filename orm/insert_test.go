package orm

import (
	"Soil/orm/internal/errs"
	"context"
	"database/sql"
	"github.com/DATA-DOG/go-sqlmock"
	_ "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestInsert_Build(t *testing.T) {
	mockDB, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	db, err := OpenDB(mockDB)
	if err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		name      string
		inserter  QueryBuilder
		wantErr   error
		wantQuery *Query
	}{
		{
			name:     "zero insert",
			inserter: NewInserter[TestModel](db).Values(),
			wantErr:  errs.ErrInsertZeroRow,
		},
		{
			name: "single insert",
			inserter: NewInserter[TestModel](db).Values(&TestModel{
				Id:        int64(1),
				FirstName: "John",
				Age:       uint8(18),
				LastName:  "Sam",
			}),
			wantQuery: &Query{
				SQL:  "INSERT INTO `test_model`(`id`,`first_name`,`age`,`last_name`) VALUES (?,?,?,?);",
				Args: []any{int64(1), "John", uint8(18), "Sam"},
			},
		},
		{
			name: "multiple insert",
			inserter: NewInserter[TestModel](db).Values(&TestModel{
				Id:        int64(1),
				FirstName: "John",
				Age:       uint8(18),
				LastName:  "Sam",
			}, &TestModel{
				Id:        int64(2),
				FirstName: "Jay",
				Age:       uint8(20),
				LastName:  "Chou",
			}),
			wantQuery: &Query{
				SQL: "INSERT INTO `test_model`(`id`,`first_name`,`age`,`last_name`) VALUES (?,?,?,?),(?,?,?,?);",
				Args: []any{int64(1), "John", uint8(18), "Sam",
					int64(2), "Jay", uint8(20), "Chou"},
			},
		},
		{
			name: "single partial insert",
			inserter: NewInserter[TestModel](db).Columns("Id", "FirstName", "LastName").Values(&TestModel{
				Id:        int64(1),
				FirstName: "John",
				LastName:  "Sam",
			}),
			wantQuery: &Query{
				SQL:  "INSERT INTO `test_model`(`id`,`first_name`,`last_name`) VALUES (?,?,?);",
				Args: []any{int64(1), "John", "Sam"},
			},
		},
		{
			name: "single partial insert",
			inserter: NewInserter[TestModel](db).Columns("Id", "FirstName", "LastName").Values(&TestModel{
				Id:        int64(1),
				FirstName: "John",
				Age:       18,
				LastName:  "Sam",
			}),
			wantQuery: &Query{
				SQL:  "INSERT INTO `test_model`(`id`,`first_name`,`last_name`) VALUES (?,?,?);",
				Args: []any{int64(1), "John", "Sam"},
			},
		},
		{
			name: "single partial insert",
			inserter: NewInserter[TestModel](db).Columns("Id", "FirstName", "Age").Values(&TestModel{
				Id:        int64(1),
				FirstName: "John",
				LastName:  "Sam",
			}),
			wantQuery: &Query{
				SQL:  "INSERT INTO `test_model`(`id`,`first_name`,`age`) VALUES (?,?,?);",
				Args: []any{int64(1), "John", uint8(0)},
			},
		},
		{
			name: "upsert",
			inserter: NewInserter[TestModel](db).Values(&TestModel{
				Id:        int64(1),
				FirstName: "John",
				Age:       uint8(18),
				LastName:  "Sam",
			}, &TestModel{
				Id:        int64(2),
				FirstName: "Jay",
				Age:       uint8(20),
				LastName:  "Chou",
			}).OnDuplicateKey().Update(Assign("FirstName", "Chan")),
			wantQuery: &Query{
				SQL: "INSERT INTO `test_model`(`id`,`first_name`,`age`,`last_name`) VALUES (?,?,?,?),(?,?,?,?)" +
					" ON DUPLICATE KEY UPDATE `first_name`=?;",
				Args: []any{int64(1), "John", uint8(18), "Sam",
					int64(2), "Jay", uint8(20), "Chou", "Chan"},
			},
		},
		{
			name: "upsert use insert value",
			inserter: NewInserter[TestModel](db).Values(&TestModel{
				Id:        int64(1),
				FirstName: "John",
				Age:       uint8(18),
				LastName:  "Sam",
			}).OnDuplicateKey().Update(Col("FirstName"), Col("LastName")),
			wantQuery: &Query{
				SQL: "INSERT INTO `test_model`(`id`,`first_name`,`age`,`last_name`) VALUES (?,?,?,?)" +
					" ON DUPLICATE KEY UPDATE `first_name`=VALUES(`first_name`),`last_name`=VALUES(`last_name`);",
				Args: []any{int64(1), "John", uint8(18), "Sam"},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			query, err := tc.inserter.Build()
			assert.Equal(t, err, tc.wantErr)
			if err != nil {
				return
			}
			assert.Equal(t, tc.wantQuery, query)
		})
	}
}

func TestInsert_SQLiteDialect_Build(t *testing.T) {
	mockDB, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	db, err := OpenDB(mockDB, DBWithDialect(SQLite))
	if err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		name      string
		inserter  QueryBuilder
		wantErr   error
		wantQuery *Query
	}{
		{
			name: "upsert",
			inserter: NewInserter[TestModel](db).Values(&TestModel{
				Id:        int64(1),
				FirstName: "John",
				Age:       uint8(18),
				LastName:  "Sam",
			}, &TestModel{
				Id:        int64(2),
				FirstName: "Jay",
				Age:       uint8(20),
				LastName:  "Chou",
			}).OnDuplicateKey().ConflictColumns("Id").Update(Assign("FirstName", "Chan")),
			wantQuery: &Query{
				SQL: "INSERT INTO `test_model`(`id`,`first_name`,`age`,`last_name`) VALUES (?,?,?,?),(?,?,?,?)" +
					" ON CONFLICT(`id`) DO UPDATE SET `first_name`=?;",
				Args: []any{int64(1), "John", uint8(18), "Sam",
					int64(2), "Jay", uint8(20), "Chou", "Chan"},
			},
		},
		{
			name: "upsert use insert value",
			inserter: NewInserter[TestModel](db).Values(&TestModel{
				Id:        int64(1),
				FirstName: "John",
				Age:       uint8(18),
				LastName:  "Sam",
			}).OnDuplicateKey().ConflictColumns("FirstName", "LastName").Update(Col("FirstName"), Col("LastName")),
			wantQuery: &Query{
				SQL: "INSERT INTO `test_model`(`id`,`first_name`,`age`,`last_name`) VALUES (?,?,?,?)" +
					" ON CONFLICT(`first_name`,`last_name`) DO UPDATE SET `first_name`=excluded.`first_name`,`last_name`=excluded.`last_name`;",
				Args: []any{int64(1), "John", uint8(18), "Sam"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			query, err := tc.inserter.Build()
			assert.Equal(t, err, tc.wantErr)
			if err != nil {
				return
			}
			assert.Equal(t, tc.wantQuery, query)
		})
	}
}

func TestInserter_Exec(t *testing.T) {
	type TestModel struct {
		Id        int64
		FirstName string
		Age       uint8
		LastName  string
	}
	dbSql, err := sql.Open("mysql", "root:yyc167943@tcp(192.168.146.128:3306)/test?charset=utf8")
	require.NoError(t, err)
	db, err := OpenDB(dbSql)
	require.NoError(t, err)
	defer func() {
		if err := dbSql.Close(); err != nil {
			t.Error(err)
		}
	}()
	testCases := []struct {
		name     string
		inserter *Inserter[TestModel]
		wantErr  error
		affected int64
	}{
		{
			name: "insert simple exec",
			inserter: NewInserter[TestModel](db).Values(&TestModel{
				Id:        int64(2),
				FirstName: "John",
				Age:       uint8(18),
				LastName:  "Sam",
			}),
			affected: int64(1),
		},
		{
			name: "insert upsert exec",
			inserter: NewInserter[TestModel](db).Values(&TestModel{
				Id:        int64(1),
				FirstName: "John",
				Age:       uint8(18),
				LastName:  "Sam",
			}).OnDuplicateKey().ConflictColumns("Id").Update(Assign("FirstName", "Chan")),
			affected: int64(2), // ON DUPLICATE KEY UPDATE，它将一个插入和一个更新操作都视为影响了行,这里的值是2
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			res := tc.inserter.Exec(context.Background())
			affectRows, er := res.RowsAffected()
			assert.Equal(t, er, tc.wantErr)
			if er != nil {
				return
			}
			assert.Equal(t, tc.affected, affectRows)
		})
	}
}
