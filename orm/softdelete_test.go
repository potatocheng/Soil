package orm

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SoftDeleteModel 带软删除与自动时间戳字段的测试模型。
// DeletedAt 使用 *time.Time，NULL 表示未删除。
type SoftDeleteModel struct {
	Id        int64
	FirstName string
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
}

// SoftDeletePtrModel 使用 *time.Time 的 CreatedAt/UpdatedAt，验证指针字段类型也能被自动填充。
type SoftDeletePtrModel struct {
	Id        int64
	FirstName string
	CreatedAt *time.Time
	UpdatedAt *time.Time
	DeletedAt *time.Time
}

// SoftDeleteTagModel 通过 orm tag 标记时间戳/软删除字段（字段名非默认名），
// 验证 tag 覆盖名识别逻辑。列名通过 column tag 指定为 ctime/utime/dtime。
type SoftDeleteTagModel struct {
	Id    int64
	Name  string
	Ctime time.Time  `orm:"column(ctime);created_at()"`
	Utime time.Time  `orm:"column(utime);updated_at()"`
	Dtime *time.Time `orm:"column(dtime);deleted_at()"`
}

// ---- Selector.Build 软删除过滤测试 ----

func TestSelector_Build_SoftDelete(t *testing.T) {
	mockDB, _, err := sqlmock.New()
	require.NoError(t, err)
	db, err := OpenDB(mockDB)
	require.NoError(t, err)

	testCases := []struct {
		name      string
		selector  QueryBuilder
		wantQuery *Query
		wantErr   error
	}{
		{
			name:     "no where, soft delete filter auto appended",
			selector: NewSelector[SoftDeleteModel](db),
			wantQuery: &Query{
				SQL: "SELECT * FROM `soft_delete_model` WHERE `deleted_at` IS NULL;",
			},
		},
		{
			name:     "with user where, combined with soft delete",
			selector: NewSelector[SoftDeleteModel](db).Where(Col("Id").EQ(int64(1))),
			wantQuery: &Query{
				SQL:  "SELECT * FROM `soft_delete_model` WHERE `deleted_at` IS NULL AND `id` = ?;",
				Args: []any{int64(1)},
			},
		},
		{
			name: "with multiple user where, combined with soft delete",
			selector: NewSelector[SoftDeleteModel](db).
				Where(Col("Id").EQ(int64(1)), Col("FirstName").EQ("Tom")),
			wantQuery: &Query{
				SQL:  "SELECT * FROM `soft_delete_model` WHERE `deleted_at` IS NULL AND (`id` = ?) AND (`first_name` = ?);",
				Args: []any{int64(1), "Tom"},
			},
		},
		{
			name:     "tag-marked model uses custom deleted_at column name",
			selector: NewSelector[SoftDeleteTagModel](db),
			wantQuery: &Query{
				SQL: "SELECT * FROM `soft_delete_tag_model` WHERE `dtime` IS NULL;",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			q, err := tc.selector.Build()
			assert.Equal(t, tc.wantErr, err)
			if err != nil {
				return
			}
			assert.Equal(t, tc.wantQuery, q)
		})
	}
}

// ---- Selector.Build 无软删除字段时向后兼容 ----

func TestSelector_Build_NoSoftDeleteField_Unchanged(t *testing.T) {
	mockDB, _, err := sqlmock.New()
	require.NoError(t, err)
	db, err := OpenDB(mockDB)
	require.NoError(t, err)

	// TestModel 无 CreatedAt/UpdatedAt/DeletedAt 字段，行为应保持不变
	q, err := NewSelector[TestModel](db).Build()
	require.NoError(t, err)
	assert.Equal(t, "SELECT * FROM `test_model`;", q.SQL)
	assert.Empty(t, q.Args)

	q2, err := NewSelector[TestModel](db).Where(Col("Id").EQ(int64(1))).Build()
	require.NoError(t, err)
	assert.Equal(t, "SELECT * FROM `test_model` WHERE `id` = ?;", q2.SQL)
	assert.Equal(t, []any{int64(1)}, q2.Args)
}

// ---- Inserter.Exec 自动填充 CreatedAt 测试 ----

func TestInserter_Exec_FillsCreatedAt(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	db, err := OpenDB(mockDB)
	require.NoError(t, err)

	t.Run("time.Time field filled", func(t *testing.T) {
		m := &SoftDeleteModel{Id: 1, FirstName: "John"}
		// 不校验具体时间值（time.Now 非确定），由后续断言 m.CreatedAt 非零来验证填充
		mock.ExpectExec("INSERT INTO `soft_delete_model`").
			WillReturnResult(sqlmock.NewResult(1, 1))

		res := NewInserter[SoftDeleteModel](db).Values(m).Exec(context.Background())
		require.NoError(t, res.Err())

		// CreatedAt 应被填充为非零值
		assert.False(t, m.CreatedAt.IsZero(), "CreatedAt 应被自动填充")
		// UpdatedAt 不在 INSERT 路径填充，应保持零值
		assert.True(t, m.UpdatedAt.IsZero(), "UpdatedAt 不应在 INSERT 时被填充")
		// DeletedAt 应保持 nil（未删除）
		assert.Nil(t, m.DeletedAt)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("pointer time.Time field filled", func(t *testing.T) {
		m := &SoftDeletePtrModel{Id: 2, FirstName: "Tom"}
		mock.ExpectExec("INSERT INTO `soft_delete_ptr_model`").
			WillReturnResult(sqlmock.NewResult(2, 1))

		res := NewInserter[SoftDeletePtrModel](db).Values(m).Exec(context.Background())
		require.NoError(t, res.Err())

		// CreatedAt 指针应被设为非 nil，且指向的时间非零
		require.NotNil(t, m.CreatedAt, "CreatedAt 指针应被自动填充为非 nil")
		assert.False(t, m.CreatedAt.IsZero())
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("batch values all filled", func(t *testing.T) {
		m1 := &SoftDeleteModel{Id: 3, FirstName: "A"}
		m2 := &SoftDeleteModel{Id: 4, FirstName: "B"}
		mock.ExpectExec("INSERT INTO `soft_delete_model`").
			WillReturnResult(sqlmock.NewResult(0, 2))

		res := NewInserter[SoftDeleteModel](db).Values(m1, m2).Exec(context.Background())
		require.NoError(t, res.Err())

		assert.False(t, m1.CreatedAt.IsZero(), "m1.CreatedAt 应被填充")
		assert.False(t, m2.CreatedAt.IsZero(), "m2.CreatedAt 应被填充")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("non-zero CreatedAt respected, not overridden", func(t *testing.T) {
		fixed := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
		m := &SoftDeleteModel{Id: 5, FirstName: "Keep", CreatedAt: fixed}
		mock.ExpectExec("INSERT INTO `soft_delete_model`").
			WillReturnResult(sqlmock.NewResult(5, 1))

		res := NewInserter[SoftDeleteModel](db).Values(m).Exec(context.Background())
		require.NoError(t, res.Err())

		// 用户预设的 CreatedAt 不应被覆盖
		assert.True(t, m.CreatedAt.Equal(fixed), "预设的 CreatedAt 不应被覆盖")
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

// ---- Inserter.Build 直接调用不填充 CreatedAt（向后兼容） ----

func TestInserter_Build_DoesNotFillCreatedAt(t *testing.T) {
	mockDB, _, err := sqlmock.New()
	require.NoError(t, err)
	db, err := OpenDB(mockDB)
	require.NoError(t, err)

	m := &SoftDeleteModel{Id: 1, FirstName: "John"}
	// 直接调用 Build（不经过 Exec），CreatedAt 不会被自动填充，应保持零值
	q, err := NewInserter[SoftDeleteModel](db).Values(m).Build()
	require.NoError(t, err)

	// SQL 结构正常（含 created_at 列），但 args 中 CreatedAt 为零值 time.Time{}
	assert.Equal(t,
		"INSERT INTO `soft_delete_model`(`id`,`first_name`,`created_at`,`updated_at`,`deleted_at`) VALUES (?,?,?,?,?);",
		q.SQL)
	assert.Len(t, q.Args, 5)
	assert.Equal(t, int64(1), q.Args[0])
	assert.Equal(t, "John", q.Args[1])
	// CreatedAt 为零值 time.Time{}
	assert.Equal(t, time.Time{}, q.Args[2])
	// 模型实例未被修改
	assert.True(t, m.CreatedAt.IsZero())
}

// ---- Updater.Build 软删除过滤测试 ----

func TestUpdater_Build_SoftDelete(t *testing.T) {
	mockDB, _, err := sqlmock.New()
	require.NoError(t, err)
	db, err := OpenDB(mockDB)
	require.NoError(t, err)

	t.Run("set assign with soft delete filter", func(t *testing.T) {
		q, err := NewUpdater[SoftDeleteModel](db).
			Set(Assign("FirstName", "Jay")).
			Where(Col("Id").EQ(int64(1))).
			Build()
		require.NoError(t, err)
		assert.Equal(t,
			"UPDATE `soft_delete_model` SET `first_name`=? WHERE `deleted_at` IS NULL AND `id` = ?;",
			q.SQL)
		assert.Equal(t, []any{"Jay", int64(1)}, q.Args)
	})

	t.Run("set assign without user where, only soft delete filter", func(t *testing.T) {
		q, err := NewUpdater[SoftDeleteModel](db).
			Set(Assign("FirstName", "Jay")).
			Build()
		require.NoError(t, err)
		assert.Equal(t,
			"UPDATE `soft_delete_model` SET `first_name`=? WHERE `deleted_at` IS NULL;",
			q.SQL)
		assert.Equal(t, []any{"Jay"}, q.Args)
	})

	t.Run("update val with soft delete filter", func(t *testing.T) {
		m := &SoftDeleteModel{Id: 1, FirstName: "Jay"}
		q, err := NewUpdater[SoftDeleteModel](db).
			Update(m).
			Set(Col("Id"), Col("FirstName")).
			Build()
		require.NoError(t, err)
		// Build 直接调用时 UpdatedAt 未被填充（在 Exec 中填充），args 为模型当前值
		assert.Equal(t,
			"UPDATE `soft_delete_model` SET `id`=?,`first_name`=? WHERE `deleted_at` IS NULL;",
			q.SQL)
		assert.Equal(t, []any{int64(1), "Jay"}, q.Args)
	})
}

// ---- Updater.Exec 自动填充 UpdatedAt 测试 ----

func TestUpdater_Exec_FillsUpdatedAt(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	db, err := OpenDB(mockDB)
	require.NoError(t, err)

	t.Run("update val fills UpdatedAt via reflection", func(t *testing.T) {
		m := &SoftDeleteModel{Id: 1, FirstName: "Jay"}
		mock.ExpectExec("UPDATE `soft_delete_model`").
			WillReturnResult(sqlmock.NewResult(0, 1))

		res := NewUpdater[SoftDeleteModel](db).
			Update(m).
			Set(Col("Id"), Col("FirstName")).
			Exec(context.Background())
		require.NoError(t, res.Err())

		// UpdatedAt 应被填充为非零值
		assert.False(t, m.UpdatedAt.IsZero(), "UpdatedAt 应被自动填充")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("set assign batch update appends UpdatedAt assign", func(t *testing.T) {
		// 仅使用 Set(Assign(...))，无模型实例，UpdatedAt 通过追加 Assign 实现
		mock.ExpectExec("UPDATE `soft_delete_model`").
			WillReturnResult(sqlmock.NewResult(0, 1))

		res := NewUpdater[SoftDeleteModel](db).
			Set(Assign("FirstName", "Jay")).
			Where(Col("Id").EQ(1)).
			Exec(context.Background())
		require.NoError(t, res.Err())
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("explicit UpdatedAt assign not duplicated", func(t *testing.T) {
		// 用户显式 Assign UpdatedAt，不应重复追加
		fixed := time.Date(2021, 6, 15, 0, 0, 0, 0, time.UTC)
		mock.ExpectExec("UPDATE `soft_delete_model`").
			WillReturnResult(sqlmock.NewResult(0, 1))

		res := NewUpdater[SoftDeleteModel](db).
			Set(Assign("FirstName", "Jay"), Assign("UpdatedAt", fixed)).
			Where(Col("Id").EQ(1)).
			Exec(context.Background())
		require.NoError(t, res.Err())
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

// ---- Updater 无软删除字段时向后兼容 ----

func TestUpdater_Build_NoSoftDeleteField_Unchanged(t *testing.T) {
	mockDB, _, err := sqlmock.New()
	require.NoError(t, err)
	db, err := OpenDB(mockDB)
	require.NoError(t, err)

	q, err := NewUpdater[TestModel](db).Set(Assign("FirstName", "Jay")).Build()
	require.NoError(t, err)
	assert.Equal(t, "UPDATE `test_model` SET `first_name`=?;", q.SQL)
	assert.Equal(t, []any{"Jay"}, q.Args)
}

// ---- Deleter.Build 软删除改写测试 ----

func TestDeleter_Build_SoftDeleteRewrite(t *testing.T) {
	mockDB, _, err := sqlmock.New()
	require.NoError(t, err)
	db, err := OpenDB(mockDB)
	require.NoError(t, err)

	t.Run("rewrite to update without user where", func(t *testing.T) {
		q, err := NewDeleter[SoftDeleteModel](db).Build()
		require.NoError(t, err)
		assert.Equal(t,
			"UPDATE `soft_delete_model` SET `deleted_at`=? WHERE `deleted_at` IS NULL;",
			q.SQL)
		require.Len(t, q.Args, 1)
		_, ok := q.Args[0].(time.Time)
		assert.True(t, ok, "第一个参数应为 time.Time")
	})

	t.Run("rewrite to update with user where", func(t *testing.T) {
		q, err := NewDeleter[SoftDeleteModel](db).Where(Col("Id").EQ(int64(1))).Build()
		require.NoError(t, err)
		assert.Equal(t,
			"UPDATE `soft_delete_model` SET `deleted_at`=? WHERE `deleted_at` IS NULL AND `id` = ?;",
			q.SQL)
		require.Len(t, q.Args, 2)
		_, ok := q.Args[0].(time.Time)
		assert.True(t, ok, "第一个参数应为 time.Time")
		assert.Equal(t, int64(1), q.Args[1])
	})

	t.Run("tag-marked model uses custom column name in rewrite", func(t *testing.T) {
		q, err := NewDeleter[SoftDeleteTagModel](db).Where(Col("Id").EQ(int64(2))).Build()
		require.NoError(t, err)
		assert.Equal(t,
			"UPDATE `soft_delete_tag_model` SET `dtime`=? WHERE `dtime` IS NULL AND `id` = ?;",
			q.SQL)
		require.Len(t, q.Args, 2)
		_, ok := q.Args[0].(time.Time)
		assert.True(t, ok)
		assert.Equal(t, int64(2), q.Args[1])
	})

	t.Run("from custom table name in rewrite", func(t *testing.T) {
		q, err := NewDeleter[SoftDeleteModel](db).From("custom_table").Where(Col("Id").EQ(int64(3))).Build()
		require.NoError(t, err)
		assert.Equal(t,
			"UPDATE custom_table SET `deleted_at`=? WHERE `deleted_at` IS NULL AND `id` = ?;",
			q.SQL)
	})
}

// ---- Deleter 无软删除字段时执行物理删除（向后兼容） ----

func TestDeleter_Build_NoSoftDeleteField_PhysicalDelete(t *testing.T) {
	mockDB, _, err := sqlmock.New()
	require.NoError(t, err)
	db, err := OpenDB(mockDB)
	require.NoError(t, err)

	q, err := NewDeleter[TestModel](db).Build()
	require.NoError(t, err)
	assert.Equal(t, "DELETE FROM `test_model`;", q.SQL)
	assert.Empty(t, q.Args)

	q2, err := NewDeleter[TestModel](db).Where(Col("Id").EQ(int64(1))).Build()
	require.NoError(t, err)
	assert.Equal(t, "DELETE FROM `test_model` WHERE `id` = ?;", q2.SQL)
	assert.Equal(t, []any{int64(1)}, q2.Args)
}

// ---- Deleter.Exec 软删除改写执行测试 ----

func TestDeleter_Exec_SoftDeleteRewrite(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	db, err := OpenDB(mockDB)
	require.NoError(t, err)

	mock.ExpectExec("UPDATE `soft_delete_model` SET `deleted_at`").
		WithArgs(sqlmock.AnyArg(), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	res := NewDeleter[SoftDeleteModel](db).Where(Col("Id").EQ(int64(1))).Exec(context.Background())
	require.NoError(t, res.Err())
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ---- tag 标记识别测试（registry 层面） ----

func TestRegistry_RecognizesTimestampAndSoftDeleteFields(t *testing.T) {
	mockDB, _, err := sqlmock.New()
	require.NoError(t, err)
	db, err := OpenDB(mockDB)
	require.NoError(t, err)

	t.Run("default field names recognized", func(t *testing.T) {
		m, err := db.r.Get(&SoftDeleteModel{})
		require.NoError(t, err)
		require.NotNil(t, m.CreatedAtField, "CreatedAtField 应被识别")
		require.NotNil(t, m.UpdatedAtField, "UpdatedAtField 应被识别")
		require.NotNil(t, m.DeletedAtField, "DeletedAtField 应被识别")
		assert.Equal(t, "CreatedAt", m.CreatedAtField.GoName)
		assert.Equal(t, "created_at", m.CreatedAtField.ColName)
		assert.Equal(t, "deleted_at", m.DeletedAtField.ColName)
	})

	t.Run("tag-marked fields recognized", func(t *testing.T) {
		m, err := db.r.Get(&SoftDeleteTagModel{})
		require.NoError(t, err)
		require.NotNil(t, m.CreatedAtField, "CreatedAtField 应通过 tag 识别")
		require.NotNil(t, m.UpdatedAtField, "UpdatedAtField 应通过 tag 识别")
		require.NotNil(t, m.DeletedAtField, "DeletedAtField 应通过 tag 识别")
		assert.Equal(t, "Ctime", m.CreatedAtField.GoName)
		assert.Equal(t, "ctime", m.CreatedAtField.ColName, "应使用 column tag 指定的列名")
		assert.Equal(t, "utime", m.UpdatedAtField.ColName)
		assert.Equal(t, "dtime", m.DeletedAtField.ColName)
	})

	t.Run("no timestamp fields, all nil", func(t *testing.T) {
		m, err := db.r.Get(&TestModel{})
		require.NoError(t, err)
		assert.Nil(t, m.CreatedAtField)
		assert.Nil(t, m.UpdatedAtField)
		assert.Nil(t, m.DeletedAtField)
	})
}
