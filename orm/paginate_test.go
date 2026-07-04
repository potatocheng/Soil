package orm

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSelectorPaginate_Build 覆盖 Task 8.1 的分页助手。
//   - TR-20: Paginate(2, 10) 生成 LIMIT ? OFFSET ?，args=[10,10]（先 limit 后 offset）
//   - TR-21: Paginate(1, 10) 生成 LIMIT ?（offset=0 时不输出 OFFSET），args=[10]
//   - page<1 按 page=1 处理
//   - size<=0 按 size=10 处理
func TestSelectorPaginate_Build(t *testing.T) {
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
			name:     "TR-20 page 2 size 10",
			selector: NewSelector[TestModel](db).Paginate(2, 10),
			wantQuery: &Query{
				SQL:  "SELECT * FROM `test_model` LIMIT ? OFFSET ?;",
				Args: []any{10, 10},
			},
		},
		{
			name:     "TR-21 page 1 size 10 (offset 0 不输出 OFFSET)",
			selector: NewSelector[TestModel](db).Paginate(1, 10),
			wantQuery: &Query{
				SQL:  "SELECT * FROM `test_model` LIMIT ?;",
				Args: []any{10},
			},
		},
		{
			name:     "page 3 size 20",
			selector: NewSelector[TestModel](db).Paginate(3, 20),
			wantQuery: &Query{
				SQL:  "SELECT * FROM `test_model` LIMIT ? OFFSET ?;",
				Args: []any{20, 40},
			},
		},
		{
			name:     "page<1 当 page=1 处理 (offset 0)",
			selector: NewSelector[TestModel](db).Paginate(0, 10),
			wantQuery: &Query{
				SQL:  "SELECT * FROM `test_model` LIMIT ?;",
				Args: []any{10},
			},
		},
		{
			name:     "page<1 当 page=1 处理 (负数)",
			selector: NewSelector[TestModel](db).Paginate(-5, 10),
			wantQuery: &Query{
				SQL:  "SELECT * FROM `test_model` LIMIT ?;",
				Args: []any{10},
			},
		},
		{
			name:     "size<=0 当 size=10 处理",
			selector: NewSelector[TestModel](db).Paginate(2, 0),
			wantQuery: &Query{
				SQL:  "SELECT * FROM `test_model` LIMIT ? OFFSET ?;",
				Args: []any{10, 10},
			},
		},
		{
			name:     "size<=0 当 size=10 处理 (负数)",
			selector: NewSelector[TestModel](db).Paginate(3, -1),
			wantQuery: &Query{
				SQL:  "SELECT * FROM `test_model` LIMIT ? OFFSET ?;",
				Args: []any{10, 20},
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

// TestSelectorPaginate_SetsFields 验证 Paginate 内部正确设置了 limit/offset 字段，
// 与等价的 Limit+Offset 链式调用产出相同 SQL。
func TestSelectorPaginate_SetsFields(t *testing.T) {
	mockDB, _, err := sqlmock.New()
	require.NoError(t, err)
	db, err := OpenDB(mockDB)
	require.NoError(t, err)

	paginated := NewSelector[TestModel](db).Paginate(3, 15).Build
	manual := NewSelector[TestModel](db).Limit(15).Offset(30).Build

	q1, err1 := paginated()
	q2, err2 := manual()
	require.NoError(t, err1)
	require.NoError(t, err2)
	assert.Equal(t, q2.SQL, q1.SQL)
	assert.Equal(t, q2.Args, q1.Args)
}
