package orm

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- 乐观锁测试模型 ----

// OptLockModel 使用默认字段名 Version（int 类型，无 tag）。
type OptLockModel struct {
	Id        int64
	FirstName string
	Version   int
}

// OptLockTagModel 通过 orm:"version()" tag 在非默认字段名上标记版本字段。
type OptLockTagModel struct {
	Id      int64
	LockVer int `orm:"version()"`
}

// OptLockStringModel 的 Version 字段为 string 类型，非整数族应被静默跳过。
type OptLockStringModel struct {
	Id      int64
	Version string
}

// OptLockSoftDeleteModel 同时定义软删除字段与版本字段，验证两者共存。
type OptLockSoftDeleteModel struct {
	Id        int64
	FirstName string
	DeletedAt *time.Time
	Version   int
}

// OptLockHookModel 实现 BeforeUpdate/AfterUpdate 钩子，
// 用于验证乐观锁冲突时 AfterUpdate 不被调用。
type OptLockHookModel struct {
	Id        int64
	FirstName string
	Version   int
}

func (m *OptLockHookModel) BeforeUpdate(ctx context.Context) error {
	hookTrace = append(hookTrace, "before_update_optlock")
	return nil
}

func (m *OptLockHookModel) AfterUpdate(ctx context.Context) error {
	hookTrace = append(hookTrace, "after_update_optlock")
	return nil
}

// ---- 1. Version 字段识别（registry 层面） ----

// TestRegistry_RecognizesVersionField 验证 registry 能正确识别乐观锁版本字段：
//   - 默认字段名 Version（int）被识别
//   - orm:"version()" tag 覆盖字段名识别
//   - 非整数族类型（string）被静默跳过
//   - 无 Version 字段时 VersionField 为 nil
func TestRegistry_RecognizesVersionField(t *testing.T) {
	mockDB, _, err := sqlmock.New()
	require.NoError(t, err)
	db, err := OpenDB(mockDB)
	require.NoError(t, err)

	t.Run("default field name Version int", func(t *testing.T) {
		m, err := db.r.Get(&OptLockModel{})
		require.NoError(t, err)
		require.NotNil(t, m.VersionField, "VersionField 应被识别")
		assert.Equal(t, "Version", m.VersionField.GoName)
		assert.Equal(t, "version", m.VersionField.ColName)
	})

	t.Run("tag version() on non-default field", func(t *testing.T) {
		m, err := db.r.Get(&OptLockTagModel{})
		require.NoError(t, err)
		require.NotNil(t, m.VersionField, "VersionField 应通过 tag 识别")
		assert.Equal(t, "LockVer", m.VersionField.GoName)
	})

	t.Run("non-integer Version field skipped", func(t *testing.T) {
		m, err := db.r.Get(&OptLockStringModel{})
		require.NoError(t, err)
		assert.Nil(t, m.VersionField, "string 类型的 Version 字段应被静默跳过")
	})

	t.Run("no Version field", func(t *testing.T) {
		m, err := db.r.Get(&TestModel{})
		require.NoError(t, err)
		assert.Nil(t, m.VersionField, "无 Version 字段时 VersionField 应为 nil")
	})
}

// ---- 2. UPDATE Build SQL 生成 ----

// TestUpdater_Build_OptimisticLock 验证 Updater.Build 在乐观锁场景下的 SQL 生成：
//   - 全量值路径（Update(val) 无 Set）：SET 追加 version=version+1，WHERE 追加 version=?
//   - 批量更新（无 Update(val)）：不追加 version 相关子句
//   - 显式 Assign version 列：跳过自动追加 version=version+1
func TestUpdater_Build_OptimisticLock(t *testing.T) {
	mockDB, _, err := sqlmock.New()
	require.NoError(t, err)
	db, err := OpenDB(mockDB)
	require.NoError(t, err)

	t.Run("full-val path: auto-append version=version+1 and version=?", func(t *testing.T) {
		m := &OptLockModel{Id: 1, FirstName: "Jay", Version: 3}
		q, err := NewUpdater[OptLockModel](db).
			Update(m).
			Where(Col("Id").EQ(1)).
			Build()
		require.NoError(t, err)
		// SET 末尾追加 `version`=`version`+1（DB 端原子自增，非 version=3）
		assert.Contains(t, q.SQL, "`version`=`version`+1")
		// WHERE 末尾追加 `version`=?
		assert.Contains(t, q.SQL, "`version`=?")
		// args 包含当前版本值 3（readVersionFromVal 返回 int64）
		assert.Contains(t, q.Args, int64(3))
		// 精确匹配：SET 中 version 字段被跳过，仅写自增；WHERE 含 version 条件
		assert.Equal(t,
			"UPDATE `opt_lock_model` SET `id`=?,`first_name`=?,`version`=`version`+1 WHERE `id` = ? AND `version`=?;",
			q.SQL)
		assert.Equal(t, []any{int64(1), "Jay", 1, int64(3)}, q.Args)
	})

	t.Run("bulk update without val: no version append", func(t *testing.T) {
		q, err := NewUpdater[OptLockModel](db).
			Set(Assign("FirstName", "Jay")).
			Where(Col("Id").EQ(1)).
			Build()
		require.NoError(t, err)
		assert.NotContains(t, q.SQL, "`version`=`version`+1", "批量更新不应追加 version 自增")
		assert.NotContains(t, q.SQL, "`version`=?", "批量更新不应追加 version 条件")
		assert.Equal(t,
			"UPDATE `opt_lock_model` SET `first_name`=? WHERE `id` = ?;",
			q.SQL)
		assert.Equal(t, []any{"Jay", 1}, q.Args)
	})

	t.Run("explicit version assign: skip auto-append in SET", func(t *testing.T) {
		m := &OptLockModel{Id: 1, FirstName: "Jay", Version: 3}
		q, err := NewUpdater[OptLockModel](db).
			Update(m).
			Set(Assign("Version", 5)).
			Where(Col("Id").EQ(1)).
			Build()
		require.NoError(t, err)
		// 用户显式 Assign Version，SET 中不应自动追加 version=version+1
		assert.NotContains(t, q.SQL, "`version`=`version`+1", "显式 Assign 后不应自动追加自增")
		// SET 中应包含用户显式的 version=?
		assert.Contains(t, q.SQL, "SET `version`=?")
		// WHERE 仍应追加 version=? 条件（乐观锁检查仍生效）
		assert.Equal(t,
			"UPDATE `opt_lock_model` SET `version`=? WHERE `id` = ? AND `version`=?;",
			q.SQL)
		// args: [显式值5, where id=1, 当前版本3]
		assert.Equal(t, []any{5, 1, int64(3)}, q.Args)
	})
}

// ---- 3. UPDATE Exec 冲突检测 ----

// TestUpdater_Exec_OptimisticLockConflict 验证 Updater.Exec 的乐观锁冲突检测：
//   - RowsAffected()==0 时返回 ErrOptimisticLock，AfterUpdate 不被调用
//   - RowsAffected()==1 时正常返回 nil，AfterUpdate 被调用
func TestUpdater_Exec_OptimisticLockConflict(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	db, err := OpenDB(mockDB)
	require.NoError(t, err)

	t.Run("conflict: RowsAffected=0 returns ErrOptimisticLock, AfterUpdate skipped", func(t *testing.T) {
		resetHookTrace()
		m := &OptLockHookModel{Id: 1, FirstName: "Jay", Version: 3}
		// 模拟并发冲突：UPDATE 命中 0 行（version 已被并发事务修改）
		mock.ExpectExec("UPDATE `opt_lock_hook_model`").
			WillReturnResult(sqlmock.NewResult(0, 0))

		res := NewUpdater[OptLockHookModel](db).
			Update(m).
			Set(Assign("FirstName", "Jay2")).
			Where(Col("Id").EQ(1)).
			Exec(context.Background())

		// 错误应可被 errors.Is 匹配到 ErrOptimisticLock
		assert.ErrorIs(t, res.Err(), ErrOptimisticLock)
		// BeforeUpdate 在 SQL 执行前已调用；AfterUpdate 不应被调用
		assert.Equal(t, []string{"before_update_optlock"}, hookTrace,
			"冲突时 AfterUpdate 不应被调用")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("success: RowsAffected=1 returns nil, AfterUpdate called", func(t *testing.T) {
		resetHookTrace()
		m := &OptLockHookModel{Id: 1, FirstName: "Jay", Version: 3}
		mock.ExpectExec("UPDATE `opt_lock_hook_model`").
			WillReturnResult(sqlmock.NewResult(0, 1))

		res := NewUpdater[OptLockHookModel](db).
			Update(m).
			Set(Assign("FirstName", "Jay2")).
			Where(Col("Id").EQ(1)).
			Exec(context.Background())

		require.NoError(t, res.Err())
		// BeforeUpdate 与 AfterUpdate 均被调用
		assert.Equal(t,
			[]string{"before_update_optlock", "after_update_optlock"}, hookTrace)
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("conflict via errors.Is wrapped sentinel", func(t *testing.T) {
		resetHookTrace()
		m := &OptLockHookModel{Id: 2, FirstName: "Tom", Version: 7}
		mock.ExpectExec("UPDATE `opt_lock_hook_model`").
			WillReturnResult(sqlmock.NewResult(0, 0))

		res := NewUpdater[OptLockHookModel](db).
			Update(m).
			Set(Assign("FirstName", "Tom2")).
			Where(Col("Id").EQ(2)).
			Exec(context.Background())

		// 即使将错误包装一层，errors.Is 仍应匹配到 ErrOptimisticLock
		wrapped := fmt.Errorf("update failed: %w", res.Err())
		assert.ErrorIs(t, wrapped, ErrOptimisticLock)
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

// ---- 4. INSERT 自动填充初始版本号 ----

// TestInserter_Exec_AutoFillVersion 验证 Inserter.Exec 自动填充 Version=1：
//   - Version 为零值时填充为 1，SQL args 中 version 列为 1
//   - Version 非零值时不被覆盖，SQL args 中 version 列为用户显式值
func TestInserter_Exec_AutoFillVersion(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	db, err := OpenDB(mockDB)
	require.NoError(t, err)

	t.Run("zero version auto-filled to 1", func(t *testing.T) {
		m := &OptLockModel{Id: 1, FirstName: "abc"} // Version 为零值
		// Exec 会在 Build 前将 Version 填为 1，SQL args 中 version 列应为 1
		mock.ExpectExec("INSERT INTO `opt_lock_model`").
			WithArgs(int64(1), "abc", 1).
			WillReturnResult(sqlmock.NewResult(1, 1))

		res := NewInserter[OptLockModel](db).Values(m).Exec(context.Background())
		require.NoError(t, res.Err())
		assert.Equal(t, 1, m.Version, "Version 应被自动填充为 1")
		assert.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("explicit version not overridden", func(t *testing.T) {
		m := &OptLockModel{Id: 1, FirstName: "abc", Version: 5}
		mock.ExpectExec("INSERT INTO `opt_lock_model`").
			WithArgs(int64(1), "abc", 5).
			WillReturnResult(sqlmock.NewResult(1, 1))

		res := NewInserter[OptLockModel](db).Values(m).Exec(context.Background())
		require.NoError(t, res.Err())
		assert.Equal(t, 5, m.Version, "显式设置的 Version 不应被覆盖")
		assert.NoError(t, mock.ExpectationsWereMet())
	})
}

// TestInserter_Build_VersionColumnIncluded 验证 Build 路径不自动填充 Version
// （自动填充仅在 Exec 阶段发生），但 SQL 结构中包含 version 列。
func TestInserter_Build_VersionColumnIncluded(t *testing.T) {
	mockDB, _, err := sqlmock.New()
	require.NoError(t, err)
	db, err := OpenDB(mockDB)
	require.NoError(t, err)

	t.Run("Build does not auto-fill, version stays zero in args", func(t *testing.T) {
		m := &OptLockModel{Id: 1, FirstName: "abc"} // Version 为零值
		q, err := NewInserter[OptLockModel](db).Values(m).Build()
		require.NoError(t, err)
		// SQL 包含 version 列
		assert.Contains(t, q.SQL, "`version`")
		assert.Equal(t,
			"INSERT INTO `opt_lock_model`(`id`,`first_name`,`version`) VALUES (?,?,?);",
			q.SQL)
		// Build 不自动填充，version args 为零值 0
		assert.Len(t, q.Args, 3)
		assert.Equal(t, 0, q.Args[2])
		// 模型实例未被修改
		assert.Equal(t, 0, m.Version)
	})

	t.Run("Build with explicit version in args", func(t *testing.T) {
		m := &OptLockModel{Id: 1, FirstName: "abc", Version: 5}
		q, err := NewInserter[OptLockModel](db).Values(m).Build()
		require.NoError(t, err)
		assert.Equal(t,
			"INSERT INTO `opt_lock_model`(`id`,`first_name`,`version`) VALUES (?,?,?);",
			q.SQL)
		assert.Equal(t, []any{int64(1), "abc", 5}, q.Args)
	})
}

// ---- 5. 与软删除共存 ----

// TestUpdater_Build_OptimisticLockWithSoftDelete 验证模型同时定义软删除字段与版本字段时，
// UPDATE Build 生成的 SQL 同时包含 deleted_at IS NULL 与 version=? 条件，
// 且 SET 中包含 version=version+1。
func TestUpdater_Build_OptimisticLockWithSoftDelete(t *testing.T) {
	mockDB, _, err := sqlmock.New()
	require.NoError(t, err)
	db, err := OpenDB(mockDB)
	require.NoError(t, err)

	m := &OptLockSoftDeleteModel{Id: 1, FirstName: "Jay", Version: 3}
	q, err := NewUpdater[OptLockSoftDeleteModel](db).
		Update(m).
		Set(Assign("FirstName", "Jay2")).
		Where(Col("Id").EQ(1)).
		Build()
	require.NoError(t, err)

	// SET 含 version=version+1
	assert.Contains(t, q.SQL, "`version`=`version`+1")
	// WHERE 含 deleted_at IS NULL（软删除过滤）
	assert.Contains(t, q.SQL, "`deleted_at` IS NULL")
	// WHERE 含 version=?（乐观锁条件）
	assert.Contains(t, q.SQL, "`version`=?")
	// 精确匹配整条 SQL
	assert.Equal(t,
		"UPDATE `opt_lock_soft_delete_model` SET `first_name`=?,`version`=`version`+1 WHERE `deleted_at` IS NULL AND `id` = ? AND `version`=?;",
		q.SQL)
	assert.Equal(t, []any{"Jay2", 1, int64(3)}, q.Args)
}

// ---- 6. ErrOptimisticLock sentinel 匹配 ----

// TestErrOptimisticLock_SentinelMatching 验证 ErrOptimisticLock 可被 errors.Is 直接匹配，
// 且包装后（%w）仍可匹配。
func TestErrOptimisticLock_SentinelMatching(t *testing.T) {
	t.Run("direct errors.Is", func(t *testing.T) {
		assert.True(t, errors.Is(ErrOptimisticLock, ErrOptimisticLock),
			"errors.Is(ErrOptimisticLock, ErrOptimisticLock) 应为 true")
	})

	t.Run("wrapped errors.Is", func(t *testing.T) {
		wrapped := fmt.Errorf("update failed: %w", ErrOptimisticLock)
		assert.True(t, errors.Is(wrapped, ErrOptimisticLock),
			"包装后的错误仍应可被 errors.Is 匹配到 ErrOptimisticLock")
	})
}
