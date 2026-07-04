package model

import (
	"Soil/orm/internal/errs"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegister_BasicFields 验证 Register 解析模型字段：列名（驼峰转下划线）、
// 类型、Offset，以及 ColumnMap 的反向查找。
type basicModel struct {
	Id        int64
	FirstName string
	Age       int8
	CreatedAt time.Time
	UpdatedAt time.Time
}

func TestRegister_BasicFields(t *testing.T) {
	r := NewRegistry()
	m, err := r.Registry(&basicModel{})
	require.NoError(t, err)
	require.NotNil(t, m)

	// 表名（驼峰转下划线）
	assert.Equal(t, "basic_model", m.TableName)

	// 字段数
	assert.Len(t, m.Fields, 5)
	assert.Len(t, m.FieldMap, 5)
	assert.Len(t, m.ColumnMap, 5)

	// Id 字段：列名 id，类型 int64（首字段 Offset 可能为 0，不强校验）
	idField, ok := m.FieldMap["Id"]
	require.True(t, ok)
	assert.Equal(t, "id", idField.ColName)
	assert.Equal(t, "Id", idField.GoName)
	assert.Equal(t, reflect.TypeOf(int64(0)), idField.Type)

	// FirstName → first_name，非首字段 Offset 必大于 0
	firstNameField, ok := m.FieldMap["FirstName"]
	require.True(t, ok)
	assert.Equal(t, "first_name", firstNameField.ColName)
	assert.NotZero(t, firstNameField.Offset)

	// ColumnMap 反向查找
	col, ok := m.ColumnMap["first_name"]
	require.True(t, ok)
	assert.Equal(t, "FirstName", col.GoName)

	// 时间戳字段识别（按 Go 字段名）
	require.NotNil(t, m.CreatedAtField)
	assert.Equal(t, "CreatedAt", m.CreatedAtField.GoName)
	require.NotNil(t, m.UpdatedAtField)
	assert.Equal(t, "UpdatedAt", m.UpdatedAtField.GoName)
	assert.Nil(t, m.DeletedAtField)
}

// TestRegister_ColumnTag 验证通过 orm tag 指定列名。
type columnTagModel struct {
	Id   int64  `orm:"column(user_id)"`
	Name string `orm:"column(user_name)"`
	Age  int8   // 无 tag，使用 Camel2Case
}

func TestRegister_ColumnTag(t *testing.T) {
	r := NewRegistry()
	m, err := r.Registry(&columnTagModel{})
	require.NoError(t, err)

	id, ok := m.FieldMap["Id"]
	require.True(t, ok)
	assert.Equal(t, "user_id", id.ColName)

	name, ok := m.FieldMap["Name"]
	require.True(t, ok)
	assert.Equal(t, "user_name", name.ColName)

	age, ok := m.FieldMap["Age"]
	require.True(t, ok)
	assert.Equal(t, "age", age.ColName)

	// ColumnMap 包含 tag 列名，不包含默认列名
	_, ok = m.ColumnMap["user_id"]
	assert.True(t, ok)
	_, ok = m.ColumnMap["user_name"]
	assert.True(t, ok)
	_, ok = m.ColumnMap["id"]
	assert.False(t, ok)
}

// TestRegister_TimestampTags 验证通过 tag 标记时间戳字段。
type timestampTagModel struct {
	Id         int64
	CreateTime time.Time  `orm:"created_at()"`
	UpdateTime time.Time  `orm:"updated_at()"`
	DeleteTime *time.Time `orm:"deleted_at()"`
}

func TestRegister_TimestampTags(t *testing.T) {
	r := NewRegistry()
	m, err := r.Registry(&timestampTagModel{})
	require.NoError(t, err)

	require.NotNil(t, m.CreatedAtField)
	assert.Equal(t, "CreateTime", m.CreatedAtField.GoName)
	require.NotNil(t, m.UpdatedAtField)
	assert.Equal(t, "UpdateTime", m.UpdatedAtField.GoName)
	require.NotNil(t, m.DeletedAtField)
	assert.Equal(t, "DeleteTime", m.DeletedAtField.GoName)
}

// TestRegister_TableNameInterface 验证实现 TableName 接口时使用自定义表名。
type customTableModel struct {
	Id int64
}

func (c *customTableModel) TableName() string {
	return "my_custom_table"
}

func TestRegister_TableNameInterface(t *testing.T) {
	r := NewRegistry()
	m, err := r.Registry(&customTableModel{})
	require.NoError(t, err)
	assert.Equal(t, "my_custom_table", m.TableName)
}

// TestRegister_NonPointer 验证传入非指针返回 ErrPointerOnly。
func TestRegister_NonPointer(t *testing.T) {
	r := NewRegistry()
	_, err := r.Registry(basicModel{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errs.ErrPointerOnly)
}

// TestRegister_PointerToNonStruct 验证传入指向非结构体的指针返回 ErrPointerOnly。
func TestRegister_PointerToNonStruct(t *testing.T) {
	r := NewRegistry()
	val := 42
	_, err := r.Registry(&val)
	require.Error(t, err)
	assert.ErrorIs(t, err, errs.ErrPointerOnly)
}

// TestRegister_GetCaching 验证 Get 对同一类型返回缓存的同一指针。
func TestRegister_GetCaching(t *testing.T) {
	r := NewRegistry()
	m1, err := r.Get(&basicModel{})
	require.NoError(t, err)
	m2, err := r.Get(&basicModel{})
	require.NoError(t, err)
	assert.Same(t, m1, m2)
}

// TestRegister_InvalidTag_NoClosingParen 验证非法 tag（不以 ) 结尾）返回错误。
type invalidTagModel struct {
	Id   int64  `orm:"column(foo)"`
	Name string `orm:"badformat"`
}

func TestRegister_InvalidTag_NoClosingParen(t *testing.T) {
	r := NewRegistry()
	_, err := r.Registry(&invalidTagModel{})
	require.Error(t, err)
	// NewErrInvalidTagContent 不包装 sentinel，每次构造都是新实例，用字符串比对
	assert.True(t, strings.Contains(err.Error(), "badformat"),
		"err.Error() = %q, want it to contain %q", err.Error(), "badformat")
}

// TestRegister_InvalidTag_MultiParen 验证非法 tag（拆分后段数 != 2）返回错误。
type invalidTagModel2 struct {
	Id int64 `orm:"col(())"`
}

func TestRegister_InvalidTag_MultiParen(t *testing.T) {
	r := NewRegistry()
	_, err := r.Registry(&invalidTagModel2{})
	require.Error(t, err)
	// "col(())" 经 strings.Split("col(())", "(") 得到 3 段，触发 len(pair) != 2 分支
	// NewErrInvalidTagContent 不包装 sentinel，用字符串比对
	assert.True(t, strings.Contains(err.Error(), "col(())"),
		"err.Error() = %q, want it to contain %q", err.Error(), "col(())")
}

// TestRegister_AnonymousField 验证匿名字段处理：parseModel 不递归展开，
// 匿名嵌入字段作为普通字段出现，GoName 为类型名。
type embedded struct {
	Extra string
}

type embeddingModel struct {
	embedded // 匿名字段
	Id       int64
}

func TestRegister_AnonymousField(t *testing.T) {
	r := NewRegistry()
	m, err := r.Registry(&embeddingModel{})
	require.NoError(t, err)

	fd, ok := m.FieldMap["embedded"]
	require.True(t, ok)
	assert.Equal(t, "embedded", fd.ColName) // Camel2Case("embedded") = "embedded"
	assert.Equal(t, reflect.TypeOf(embedded{}), fd.Type)

	_, ok = m.FieldMap["Id"]
	assert.True(t, ok)
	assert.Len(t, m.Fields, 2)
}

// TestWithTableName 验证 WithTableName opt 覆盖默认表名。
func TestWithTableName(t *testing.T) {
	r := NewRegistry()
	m, err := r.Registry(&basicModel{}, WithTableName("override_table"))
	require.NoError(t, err)
	assert.Equal(t, "override_table", m.TableName)
}

// TestWithColumName_Success 验证 WithColumName opt 重命名列成功。
func TestWithColumName_Success(t *testing.T) {
	r := NewRegistry()
	m, err := r.Registry(&basicModel{}, WithColumName("FirstName", "f_name"))
	require.NoError(t, err)

	fd, ok := m.FieldMap["FirstName"]
	require.True(t, ok)
	assert.Equal(t, "f_name", fd.ColName)

	col, ok := m.ColumnMap["f_name"]
	require.True(t, ok)
	assert.Equal(t, "FirstName", col.GoName)

	// 旧列名应已从 ColumnMap 中删除
	_, ok = m.ColumnMap["first_name"]
	assert.False(t, ok)
}

// TestWithColumName_UnknownField 验证 WithColumName 对未知字段返回 NewErrUnknownField。
func TestWithColumName_UnknownField(t *testing.T) {
	r := NewRegistry()
	_, err := r.Registry(&basicModel{}, WithColumName("NotExist", "x"))
	require.Error(t, err)
	// NewErrUnknownField 不包装 sentinel，每次构造都是新实例，用字符串比对
	assert.True(t, strings.Contains(err.Error(), "NotExist"),
		"err.Error() = %q, want it to contain %q", err.Error(), "NotExist")
}

// TestWithColumName_UnknownColumn 验证 WithColumName 在 FieldMap 与 ColumnMap
// 不一致（FieldMap 中字段对应的 ColName 不在 ColumnMap 中）时返回
// NewErrUnknownColumn。该分支是防御性检查，正常 parseModel 流程下不可达，
// 故手工构造不一致状态触发。
func TestWithColumName_UnknownColumn(t *testing.T) {
	m := &Model{
		FieldMap:  map[string]*Field{"Foo": {ColName: "foo"}},
		ColumnMap: map[string]*Field{}, // 故意留空，制造不一致
	}
	err := WithColumName("Foo", "bar")(m)
	require.Error(t, err)
	// NewErrUnknownColumn 不包装 sentinel，每次构造都是新实例，用字符串比对
	assert.True(t, strings.Contains(err.Error(), "Foo"),
		"err.Error() = %q, want it to contain %q", err.Error(), "Foo")
}
