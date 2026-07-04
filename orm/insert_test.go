package orm

import (
	"Soil/orm/internal/errs"
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	_ "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			name: "single partial insert with zero age",
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

//func TestInserter_Exec(t *testing.T) {
//	type TestModel struct {
//		Id        int64
//		FirstName string
//		Age       uint8
//		LastName  string
//	}
//	dbSql, err := sql.Open("mysql", "root:yyc167943@tcp(192.168.146.128:3306)/test?charset=utf8")
//	require.NoError(t, err)
//	db, err := OpenDB(dbSql)
//	require.NoError(t, err)
//	defer func() {
//		if err := dbSql.Close(); err != nil {
//			t.Error(err)
//		}
//	}()
//	testCases := []struct {
//		name     string
//		inserter *Inserter[TestModel]
//		wantErr  error
//		affected int64
//	}{
//		{
//			name: "insert simple exec",
//			inserter: NewInserter[TestModel](db).Values(&TestModel{
//				Id:        int64(2),
//				FirstName: "John",
//				Age:       uint8(18),
//				LastName:  "Sam",
//			}),
//			affected: int64(1),
//		},
//		{
//			name: "insert upsert exec",
//			inserter: NewInserter[TestModel](db).Values(&TestModel{
//				Id:        int64(1),
//				FirstName: "John",
//				Age:       uint8(18),
//				LastName:  "Sam",
//			}).OnDuplicateKey().ConflictColumns("Id").Update(Assign("FirstName", "Chan")),
//			affected: int64(2), // ON DUPLICATE KEY UPDATE，它将一个插入和一个更新操作都视为影响了行,这里的值是2
//		},
//	}
//
//	for _, tc := range testCases {
//		t.Run(tc.name, func(t *testing.T) {
//			res := tc.inserter.Exec(context.Background())
//			affectRows, er := res.RowsAffected()
//			assert.Equal(t, er, tc.wantErr)
//			if er != nil {
//				return
//			}
//			assert.Equal(t, tc.affected, affectRows)
//		})
//	}
//}

// ---- Task 8.2: 批量 INSERT 分块测试 ----

// TestInserter_ChunkSize 验证 ChunkSize 链式方法正确设置 chunkSize 字段。
func TestInserter_ChunkSize(t *testing.T) {
	mockDB, _, err := sqlmock.New()
	require.NoError(t, err)
	db, err := OpenDB(mockDB)
	require.NoError(t, err)

	ins := NewInserter[TestModel](db)
	assert.Equal(t, 0, ins.chunkSize, "默认 chunkSize 应为 0（不分块）")

	ins2 := NewInserter[TestModel](db).ChunkSize(50)
	assert.Equal(t, 50, ins2.chunkSize)

	// 链式调用应返回同一个 Inserter 指针。
	ins3 := NewInserter[TestModel](db)
	assert.Same(t, ins3, ins3.ChunkSize(7))
}

// TestSplitChunk 直接测试分片辅助函数，覆盖各种边界。
func TestSplitChunk(t *testing.T) {
	testCases := []struct {
		name   string
		values []int
		size   int
		want   [][]int
	}{
		{
			name:   "size<=0 返回单个分块",
			values: []int{1, 2, 3},
			size:   0,
			want:   [][]int{{1, 2, 3}},
		},
		{
			name:   "size>len 返回单个分块",
			values: []int{1, 2},
			size:   5,
			want:   [][]int{{1, 2}},
		},
		{
			name:   "正好整除",
			values: []int{1, 2, 3, 4},
			size:   2,
			want:   [][]int{{1, 2}, {3, 4}},
		},
		{
			name:   "非整除，最后一批较小",
			values: []int{1, 2, 3, 4, 5},
			size:   2,
			want:   [][]int{{1, 2}, {3, 4}, {5}},
		},
		{
			name:   "size=1 每个元素一批",
			values: []int{1, 2, 3},
			size:   1,
			want:   [][]int{{1}, {2}, {3}},
		},
		{
			name:   "空切片返回单个空分块",
			values: []int{},
			size:   2,
			want:   [][]int{{}},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := splitChunk(tc.values, tc.size)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestInsert_ChunkedExec 验证分块执行：5 个值 chunkSize=2，期望 3 次 Exec（2+2+1），
// 每次匹配对应行数的 SQL，汇总影响行数为 5。
func TestInsert_ChunkedExec(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	db, err := OpenDB(mockDB)
	require.NoError(t, err)

	values := []*TestModel{
		{Id: 1, FirstName: "a", Age: 1, LastName: "A"},
		{Id: 2, FirstName: "b", Age: 2, LastName: "B"},
		{Id: 3, FirstName: "c", Age: 3, LastName: "C"},
		{Id: 4, FirstName: "d", Age: 4, LastName: "D"},
		{Id: 5, FirstName: "e", Age: 5, LastName: "E"},
	}

	// chunkSize=2 → 3 批：[1,2]、[3,4]、[5]
	mock.ExpectExec("INSERT INTO `test_model`").
		WithArgs(int64(1), "a", uint8(1), "A", int64(2), "b", uint8(2), "B").
		WillReturnResult(sqlmock.NewResult(2, 2))
	mock.ExpectExec("INSERT INTO `test_model`").
		WithArgs(int64(3), "c", uint8(3), "C", int64(4), "d", uint8(4), "D").
		WillReturnResult(sqlmock.NewResult(4, 2))
	mock.ExpectExec("INSERT INTO `test_model`").
		WithArgs(int64(5), "e", uint8(5), "E").
		WillReturnResult(sqlmock.NewResult(5, 1))

	res := NewInserter[TestModel](db).Values(values...).ChunkSize(2).Exec(context.Background())
	require.NoError(t, res.Err())

	rows, err := res.RowsAffected()
	require.NoError(t, err)
	assert.Equal(t, int64(5), rows, "汇总影响行数应为 2+2+1=5")
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestInsert_ChunkedBuildSQL 验证分块时每批 Build 出的 SQL 包含正确行数。
// 不执行真实 Exec，仅断言 Build 在替换 values 后能产出对应行数的 VALUES 占位符。
func TestInsert_ChunkedBuildSQL(t *testing.T) {
	mockDB, _, err := sqlmock.New()
	require.NoError(t, err)
	db, err := OpenDB(mockDB)
	require.NoError(t, err)

	values := []*TestModel{
		{Id: 1, FirstName: "a", Age: 1, LastName: "A"},
		{Id: 2, FirstName: "b", Age: 2, LastName: "B"},
		{Id: 3, FirstName: "c", Age: 3, LastName: "C"},
	}

	ins := NewInserter[TestModel](db).Values(values...).ChunkSize(2)
	chunks := splitChunk(ins.values, ins.chunkSize)
	require.Len(t, chunks, 2, "3 个值 chunkSize=2 应分为 2 批")

	// 第 1 批 2 行
	ins.values = chunks[0]
	ins.sqlStrBuilder.Reset()
	q1, err := ins.Build()
	require.NoError(t, err)
	assert.Equal(t,
		"INSERT INTO `test_model`(`id`,`first_name`,`age`,`last_name`) VALUES (?,?,?,?),(?,?,?,?);",
		q1.SQL)
	assert.Len(t, q1.Args, 8)

	// 第 2 批 1 行
	ins.values = chunks[1]
	ins.sqlStrBuilder.Reset()
	q2, err := ins.Build()
	require.NoError(t, err)
	assert.Equal(t,
		"INSERT INTO `test_model`(`id`,`first_name`,`age`,`last_name`) VALUES (?,?,?,?);",
		q2.SQL)
	assert.Len(t, q2.Args, 4)
}

// TestInsert_ChunkedHooksCalledOncePerValue 验证分块执行时钩子在每个值上仅调用一次
// （总体一次，而非每批一次）：BeforeInsert 全部先调用，AfterInsert 全部最后调用。
func TestInsert_ChunkedHooksCalledOncePerValue(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	db, err := OpenDB(mockDB)
	require.NoError(t, err)

	resetHookTrace()
	ms := []*HookModel{
		{Id: 1, Age: 1},
		{Id: 2, Age: 2},
		{Id: 3, Age: 3},
	}

	// chunkSize=2 → 2 批：[1,2]、[3]
	// BeforeInsert 会把 FirstName 填为 "from_hook"。
	mock.ExpectExec("INSERT INTO `hook_model`").
		WithArgs(int64(1), "from_hook", 1, "", int64(2), "from_hook", 2, "").
		WillReturnResult(sqlmock.NewResult(2, 2))
	mock.ExpectExec("INSERT INTO `hook_model`").
		WithArgs(int64(3), "from_hook", 3, "").
		WillReturnResult(sqlmock.NewResult(3, 1))

	res := NewInserter[HookModel](db).Values(ms...).ChunkSize(2).Exec(context.Background())
	require.NoError(t, res.Err())

	// 3 个值：3 次 before_insert + 3 次 after_insert，且 before 全部在 after 之前。
	assert.Equal(t,
		[]string{
			"before_insert", "before_insert", "before_insert",
			"after_insert", "after_insert", "after_insert",
		},
		hookTrace)
	// 每个值的钩子修改均生效。
	for _, m := range ms {
		assert.Equal(t, "from_hook", m.FirstName)
		assert.Equal(t, "after_done", m.LastName)
	}
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestInsert_ChunkedStopsOnError 验证某批失败时停止后续批次并返回错误。
func TestInsert_ChunkedStopsOnError(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	db, err := OpenDB(mockDB)
	require.NoError(t, err)

	values := []*TestModel{
		{Id: 1, FirstName: "a", Age: 1, LastName: "A"},
		{Id: 2, FirstName: "b", Age: 2, LastName: "B"},
		{Id: 3, FirstName: "c", Age: 3, LastName: "C"},
		{Id: 4, FirstName: "d", Age: 4, LastName: "D"},
	}
	execErr := errors.New("batch failed")

	// chunkSize=2 → 第 1 批成功，第 2 批失败，不应有第 3 批期望。
	mock.ExpectExec("INSERT INTO `test_model`").
		WithArgs(int64(1), "a", uint8(1), "A", int64(2), "b", uint8(2), "B").
		WillReturnResult(sqlmock.NewResult(2, 2))
	mock.ExpectExec("INSERT INTO `test_model`").
		WithArgs(int64(3), "c", uint8(3), "C", int64(4), "d", uint8(4), "D").
		WillReturnError(execErr)

	res := NewInserter[TestModel](db).Values(values...).ChunkSize(2).Exec(context.Background())
	assert.Equal(t, execErr, res.Err())
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestInsert_ChunkSizeNotTriggeredWhenValuesLE 验证 values 数量 <= chunkSize 时走原有一次插入路径。
func TestInsert_ChunkSizeNotTriggeredWhenValuesLE(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	db, err := OpenDB(mockDB)
	require.NoError(t, err)

	values := []*TestModel{
		{Id: 1, FirstName: "a", Age: 1, LastName: "A"},
		{Id: 2, FirstName: "b", Age: 2, LastName: "B"},
	}

	// chunkSize=5 > len(values)=2，应一次插完，仅 1 次 Exec。
	mock.ExpectExec("INSERT INTO `test_model`").
		WithArgs(int64(1), "a", uint8(1), "A", int64(2), "b", uint8(2), "B").
		WillReturnResult(sqlmock.NewResult(2, 2))

	res := NewInserter[TestModel](db).Values(values...).ChunkSize(5).Exec(context.Background())
	require.NoError(t, res.Err())
	rows, err := res.RowsAffected()
	require.NoError(t, err)
	assert.Equal(t, int64(2), rows)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// 确保 errs 包在新增 import 后仍被使用（避免编译期 unused 报错）。
var _ = errs.ErrInsertZeroRow
