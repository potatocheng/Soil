package orm

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hookTrace 记录钩子调用顺序，用于验证钩子是否被触发及触发次序。
// 各测试在开始前清空它。
var hookTrace []string

func resetHookTrace() {
	hookTrace = hookTrace[:0]
}

// ---- Insert 钩子测试模型 ----

// HookModel 实现 BeforeInsert/AfterInsert。
type HookModel struct {
	Id        int64
	FirstName string
	Age       int
	LastName  string
}

func (h *HookModel) BeforeInsert(ctx context.Context) error {
	hookTrace = append(hookTrace, "before_insert")
	// 在钩子内修改字段：若 FirstName 为空则填入默认值，验证钩子对生成 SQL 的影响。
	if h.FirstName == "" {
		h.FirstName = "from_hook"
	}
	return nil
}

func (h *HookModel) AfterInsert(ctx context.Context) error {
	hookTrace = append(hookTrace, "after_insert")
	// 标记 AfterInsert 被调用（在 SQL 执行后，不影响 SQL）。
	h.LastName = "after_done"
	return nil
}

// HookInsertErrModel 的 BeforeInsert 总是返回 error。
type HookInsertErrModel struct {
	Id int64
}

var errBeforeInsert = errors.New("before insert failed")

func (h *HookInsertErrModel) BeforeInsert(ctx context.Context) error {
	hookTrace = append(hookTrace, "before_insert_err")
	return errBeforeInsert
}

// HookAfterInsertErrModel 的 AfterInsert 总是返回 error。
type HookAfterInsertErrModel struct {
	Id int64
}

var errAfterInsert = errors.New("after insert failed")

func (h *HookAfterInsertErrModel) BeforeInsert(ctx context.Context) error {
	hookTrace = append(hookTrace, "before_insert")
	return nil
}

func (h *HookAfterInsertErrModel) AfterInsert(ctx context.Context) error {
	hookTrace = append(hookTrace, "after_insert_err")
	return errAfterInsert
}

// ---- Query 钩子测试模型 ----

// HookQueryModel 实现 BeforeQuery/AfterQuery。
type HookQueryModel struct {
	Id        int64
	FirstName string
	Age       int
	LastName  string
}

func (h *HookQueryModel) BeforeQuery(ctx context.Context) error {
	hookTrace = append(hookTrace, "before_query")
	return nil
}

func (h *HookQueryModel) AfterQuery(ctx context.Context) error {
	hookTrace = append(hookTrace, "after_query")
	return nil
}

// HookQueryErrModel 的 BeforeQuery 总是返回 error。
type HookQueryErrModel struct {
	Id int64
}

var errBeforeQuery = errors.New("before query failed")

func (h *HookQueryErrModel) BeforeQuery(ctx context.Context) error {
	hookTrace = append(hookTrace, "before_query_err")
	return errBeforeQuery
}

// ---- Update 钩子测试模型 ----

// HookUpdateModel 实现 BeforeUpdate/AfterUpdate。
type HookUpdateModel struct {
	Id        int64
	FirstName string
	Age       int
	LastName  string
}

func (h *HookUpdateModel) BeforeUpdate(ctx context.Context) error {
	hookTrace = append(hookTrace, "before_update")
	if h.FirstName == "" {
		h.FirstName = "updated_by_hook"
	}
	return nil
}

func (h *HookUpdateModel) AfterUpdate(ctx context.Context) error {
	hookTrace = append(hookTrace, "after_update")
	h.LastName = "after_update"
	return nil
}

// ---- Insert 钩子测试 ----

func TestInsertHook_BeforeInsertModifiesValueAndAfterInsertCalled(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	db, err := OpenDB(mockDB)
	require.NoError(t, err)

	resetHookTrace()
	m := &HookModel{Id: 1, Age: 18} // FirstName、LastName 留空

	// BeforeInsert 会将 FirstName 设为 "from_hook"，LastName 仍为空。
	mock.ExpectExec("INSERT INTO `hook_model`").
		WithArgs(int64(1), "from_hook", 18, "").
		WillReturnResult(sqlmock.NewResult(1, 1))

	res := NewInserter[HookModel](db).Values(m).Exec(context.Background())
	require.NoError(t, res.Err())

	// 验证钩子按预期顺序触发。
	assert.Equal(t, []string{"before_insert", "after_insert"}, hookTrace)
	// AfterInsert 在 SQL 执行后修改了 LastName。
	assert.Equal(t, "after_done", m.LastName)
	// BeforeInsert 修改了 FirstName。
	assert.Equal(t, "from_hook", m.FirstName)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestInsertHook_BatchValuesCallHooksForEach(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	db, err := OpenDB(mockDB)
	require.NoError(t, err)

	resetHookTrace()
	m1 := &HookModel{Id: 1, Age: 18}
	m2 := &HookModel{Id: 2, Age: 20}

	mock.ExpectExec("INSERT INTO `hook_model`").
		WithArgs(int64(1), "from_hook", 18, "", int64(2), "from_hook", 20, "").
		WillReturnResult(sqlmock.NewResult(2, 2))

	res := NewInserter[HookModel](db).Values(m1, m2).Exec(context.Background())
	require.NoError(t, res.Err())

	// 两个元素的 Before/After 钩子都应被调用。
	assert.Equal(t,
		[]string{"before_insert", "before_insert", "after_insert", "after_insert"},
		hookTrace)
	assert.Equal(t, "from_hook", m1.FirstName)
	assert.Equal(t, "from_hook", m2.FirstName)
	assert.Equal(t, "after_done", m1.LastName)
	assert.Equal(t, "after_done", m2.LastName)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestInsertHook_BeforeInsertErrorAbortsExec(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	db, err := OpenDB(mockDB)
	require.NoError(t, err)

	resetHookTrace()
	m := &HookInsertErrModel{Id: 1}

	// 不设置任何 ExpectExec —— 若 SQL 被执行，ExpectationsWereMet 会失败。
	res := NewInserter[HookInsertErrModel](db).Values(m).Exec(context.Background())
	assert.Equal(t, errBeforeInsert, res.Err())
	assert.Equal(t, []string{"before_insert_err"}, hookTrace)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestInsertHook_AfterInsertErrorReturned(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	db, err := OpenDB(mockDB)
	require.NoError(t, err)

	resetHookTrace()
	m := &HookAfterInsertErrModel{Id: 1}

	// SQL 正常执行成功，但 AfterInsert 返回 error。
	mock.ExpectExec("INSERT INTO `hook_after_insert_err_model`").
		WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(1, 1))

	res := NewInserter[HookAfterInsertErrModel](db).Values(m).Exec(context.Background())
	assert.Equal(t, errAfterInsert, res.Err())
	assert.Equal(t, []string{"before_insert", "after_insert_err"}, hookTrace)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// TestInsertHook_NonHookModelUnaffected 验证未实现钩子的模型行为不变。
func TestInsertHook_NonHookModelUnaffected(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	db, err := OpenDB(mockDB)
	require.NoError(t, err)

	resetHookTrace()
	mock.ExpectExec("INSERT INTO `test_model`").
		WithArgs(int64(1), "John", uint8(18), "Sam").
		WillReturnResult(sqlmock.NewResult(1, 1))

	res := NewInserter[TestModel](db).Values(&TestModel{
		Id:        1,
		FirstName: "John",
		Age:       18,
		LastName:  "Sam",
	}).Exec(context.Background())
	require.NoError(t, res.Err())
	// 没有钩子被调用。
	assert.Empty(t, hookTrace)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ---- Query 钩子测试 ----

func TestSelectorHook_BeforeAndAfterQueryCalled(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	db, err := OpenDB(mockDB)
	require.NoError(t, err)

	resetHookTrace()
	mock.ExpectQuery("SELECT .*").
		WillReturnRows(sqlmock.NewRows([]string{"id", "first_name", "age", "last_name"}).
			AddRow([]byte("1"), []byte("db_fn"), []byte("18"), []byte("db_ln")))

	res, err := NewSelector[HookQueryModel](db).Get(context.Background())
	require.NoError(t, err)
	require.NotNil(t, res)

	// 钩子按预期顺序触发。
	assert.Equal(t, []string{"before_query", "after_query"}, hookTrace)
	// 结果由数据库行填充（BeforeQuery 在填充前运行，其修改会被覆盖）。
	assert.Equal(t, int64(1), res.Id)
	assert.Equal(t, "db_fn", res.FirstName)
	assert.Equal(t, 18, res.Age)
	assert.Equal(t, "db_ln", res.LastName)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSelectorHook_BeforeQueryErrorAbortsGet(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	db, err := OpenDB(mockDB)
	require.NoError(t, err)

	resetHookTrace()
	// 不设置 ExpectQuery —— 若 SQL 被执行，ExpectationsWereMet 会失败。
	res, err := NewSelector[HookQueryErrModel](db).Get(context.Background())
	assert.Equal(t, errBeforeQuery, err)
	assert.Nil(t, res)
	assert.Equal(t, []string{"before_query_err"}, hookTrace)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// ---- Update 钩子测试 ----

func TestUpdaterHook_HooksCalledWhenValSet(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	db, err := OpenDB(mockDB)
	require.NoError(t, err)

	resetHookTrace()
	m := &HookUpdateModel{Id: 1, Age: 18} // FirstName、LastName 留空

	// BeforeUpdate 会将 FirstName 设为 "updated_by_hook"。
	// 使用 Set(Col(...)) 让 Build 从 val 读取各列值。
	mock.ExpectExec("UPDATE `hook_update_model`").
		WithArgs(int64(1), "updated_by_hook", 18, "").
		WillReturnResult(sqlmock.NewResult(0, 1))

	res := NewUpdater[HookUpdateModel](db).Update(m).
		Set(Col("Id"), Col("FirstName"), Col("Age"), Col("LastName")).
		Exec(context.Background())
	require.NoError(t, res.Err())

	assert.Equal(t, []string{"before_update", "after_update"}, hookTrace)
	assert.Equal(t, "updated_by_hook", m.FirstName)
	assert.Equal(t, "after_update", m.LastName)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdaterHook_HooksSkippedWhenValNil(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	db, err := OpenDB(mockDB)
	require.NoError(t, err)

	resetHookTrace()
	// 仅使用 Set(Assign(...))，未调用 Update(val)，因此没有模型实例，钩子应被跳过。
	mock.ExpectExec("UPDATE `hook_update_model`").
		WithArgs("Jay").
		WillReturnResult(sqlmock.NewResult(0, 1))

	res := NewUpdater[HookUpdateModel](db).Set(Assign("FirstName", "Jay")).
		Exec(context.Background())
	require.NoError(t, res.Err())
	// 没有钩子被调用。
	assert.Empty(t, hookTrace)
	assert.NoError(t, mock.ExpectationsWereMet())
}
