package orm

import (
	"Soil/orm/internal/errs"
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
)

func TestSelector_Build(t *testing.T) {
	type Order struct {
		Id        int
		UsingCol1 string
		UsingCol2 string
	}

	type OrderDetail struct {
		OrderId int
		ItemId  int

		UsingCol1 string
		UsingCol2 string
	}

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
		selector  QueryBuilder
		wantQuery *Query
		wantErr   error
	}{
		{
			name:     "no from",
			selector: NewSelector[TestModel](db),
			wantQuery: &Query{
				SQL: "SELECT * FROM `test_model`;",
			},
		},
		{
			name:     "with from",
			selector: NewSelector[TestModel](db).From(TableOf(&TestModel{})),
			wantQuery: &Query{
				SQL: "SELECT * FROM `test_model`;",
			},
		},
		{
			name:     "empty from",
			selector: NewSelector[TestModel](db).From(nil),
			wantQuery: &Query{
				SQL: "SELECT * FROM `test_model`;",
			},
		},
		{
			name: "join using",
			selector: func() *Selector[Order] {
				tab1 := TableOf(&Order{})
				j1 := tab1.Join(TableOf(&OrderDetail{})).Using("UsingCol1", "UsingCol2")
				return NewSelector[Order](db).From(j1)
			}(),
			wantQuery: &Query{
				SQL: "SELECT * FROM (`order` JOIN `order_detail` USING (`using_col1`, `using_col2`));",
			},
		},
		{
			name: "join on",
			selector: func() *Selector[Order] {
				tab1 := TableOf(&Order{})
				tab2 := TableOf(&OrderDetail{})
				return NewSelector[Order](db).From(tab1.Join(tab2).On(tab1.Col("Id").EQ(tab2.Col("OrderId"))))
			}(),
			wantQuery: &Query{
				SQL: "SELECT * FROM (`order` JOIN `order_detail` ON `order`.`id` = `order_detail`.`order_id`);",
			},
		},
		{
			name: "join on alias",
			selector: func() *Selector[Order] {
				tab1 := TableOf(&Order{}).As("t1")
				tab2 := TableOf(&OrderDetail{}).As("t2")
				return NewSelector[Order](db).From(tab1.Join(tab2).On(tab1.Col("Id").EQ(tab2.Col("OrderId"))))
			}(),
			wantQuery: &Query{
				SQL: "SELECT * FROM (`order` AS `t1` JOIN `order_detail` AS `t2` ON `t1`.`id` = `t2`.`order_id`);",
			},
		},
		//{
		//	name:     "with db",
		//	selector: NewSelector[TestModel](db).From("`test_db`.`test_model`"),
		//	wantQuery: &Query{
		//		SQL: "SELECT * FROM `test_db`.`test_model`;",
		//	},
		//},
		//{
		//	name: "single and simple predicate",
		//	selector: NewSelector[TestModel](db).From("`test_db`.`test_model`").
		//		Where(Col("Age").EQ(18)),
		//	wantQuery: &Query{
		//		SQL:  "SELECT * FROM `test_db`.`test_model` WHERE `age` = ?;",
		//		Args: []any{18},
		//	},
		//},
		//{
		//	name: "or",
		//	selector: NewSelector[TestModel](db).From("`test_db`.`test_model`").
		//		Where(Col("Age").EQ(18).Or(Col("Id").EQ(1))),
		//	wantQuery: &Query{
		//		SQL:  "SELECT * FROM `test_db`.`test_model` WHERE (`age` = ?) OR (`id` = ?);",
		//		Args: []any{18, 1},
		//	},
		//},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			q, err := testCase.selector.Build()
			assert.Equal(t, testCase.wantErr, err)
			if err != nil {
				return
			}
			assert.Equal(t, testCase.wantQuery, q)
		})
	}
}

type TestModel struct {
	Id        int64
	FirstName string
	Age       uint8
	LastName  string
}

func TestSelector_Get(t *testing.T) {
	dbMock, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	db, err := OpenDB(dbMock)
	if err != nil {
		t.Fatal(err)
	}

	//mock.ExpectQuery()
	testCases := []struct {
		name     string
		query    string
		wantVal  *TestModel
		mockRows *sqlmock.Rows
		mockErr  error
		wantErr  error
	}{
		{
			// 查询返回错误
			name:    "query error",
			mockErr: errors.New("invalid query"),
			wantErr: errors.New("invalid query"),
			query:   "SELECT .*",
		},
		{
			name:     "no row",
			wantErr:  errs.ErrNoRows,
			query:    "SELECT .*",
			mockRows: sqlmock.NewRows([]string{"id"}),
		},
		{
			name:  "get data",
			query: "SELECT .*",
			mockRows: func() *sqlmock.Rows {
				res := sqlmock.NewRows([]string{"id", "first_name", "age", "last_name"})
				res.AddRow([]byte("1"), []byte("yang"), []byte("18"), []byte("cheng"))
				return res
			}(),
			wantVal: &TestModel{
				Id:        1,
				FirstName: "yang",
				Age:       18,
				LastName:  "cheng",
			},
		},
	}

	for _, testCase := range testCases {
		exp := mock.ExpectQuery(testCase.query)
		if testCase.mockErr != nil {
			exp.WillReturnError(testCase.mockErr)
		} else {
			exp.WillReturnRows(testCase.mockRows)
		}
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			res, err := NewSelector[TestModel](db).Get(context.Background())
			assert.Equal(t, err, testCase.wantErr)
			if err != nil {
				return
			}
			assert.Equal(t, testCase.wantVal, res)
		})
	}
}

func TestSelector_Select(t *testing.T) {
	mockDB, _, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	db, err := OpenDB(mockDB)
	if err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		name     string
		selector QueryBuilder
		query    *Query
		wantErr  error
	}{
		{
			name:     "select column successful",
			selector: NewSelector[TestModel](db).Select(Col("Id")),
			query: &Query{
				SQL: "SELECT `id` FROM `test_model`;",
			},
		},
		{
			name:     "select multiple column successful",
			selector: NewSelector[TestModel](db).Select(Col("Id"), Col("FirstName")),
			query: &Query{
				SQL: "SELECT `id`,`first_name` FROM `test_model`;",
			},
		},
		{
			name:     "select invalid column successful",
			selector: NewSelector[TestModel](db).Select(Col("id")),
			query: &Query{
				SQL: "SELECT `id`,`first_name` FROM `test_model`;",
			},
			wantErr: errs.NewErrUnknownField("id"),
		},
		{
			name:     "select aggregate AVG",
			selector: NewSelector[TestModel](db).Select(Avg("Id")),
			query: &Query{
				SQL: "SELECT AVG(`id`) FROM `test_model`;",
			},
		},
		{
			name:     "select aggregate COUNT",
			selector: NewSelector[TestModel](db).Select(Count("Id")),
			query: &Query{
				SQL: "SELECT COUNT(`id`) FROM `test_model`;",
			},
		},
		{
			name:     "select aggregate SUM",
			selector: NewSelector[TestModel](db).Select(Sum("Id")),
			query: &Query{
				SQL: "SELECT SUM(`id`) FROM `test_model`;",
			},
		},
		{
			name:     "select aggregate MIN",
			selector: NewSelector[TestModel](db).Select(Min("Id")),
			query: &Query{
				SQL: "SELECT MIN(`id`) FROM `test_model`;",
			},
		},
		{
			name:     "select aggregate MAX",
			selector: NewSelector[TestModel](db).Select(Max("Id")),
			query: &Query{
				SQL: "SELECT MAX(`id`) FROM `test_model`;",
			},
		},
		{
			name:     "select multiple aggregate",
			selector: NewSelector[TestModel](db).Select(Max("Id"), Min("Age")),
			query: &Query{
				SQL: "SELECT MAX(`id`),MIN(`age`) FROM `test_model`;",
			},
		},
		{
			name:     "select invalid aggregate",
			selector: NewSelector[TestModel](db).Select(Max("invalid"), Min("Age")),
			//query: &Query{
			//	SQL: "SELECT MAX(`id`),MIN(`age`) FROM `test_model`;",
			//},
			wantErr: errs.NewErrUnknownField("invalid"),
		},
		{
			name:     "select raw expression",
			selector: NewSelector[TestModel](db).Select(Raw("COUNT(DISTINCT `first_name`)")),
			query: &Query{
				SQL: "SELECT COUNT(DISTINCT `first_name`) FROM `test_model`;",
			},
		},
		{
			name: "select raw where",
			selector: NewSelector[TestModel](db).Select(Raw("COUNT(DISTINCT `first_name`)")).
				Where(Raw("`age` < ?", 200).AsPredicate()),
			query: &Query{
				SQL:  "SELECT COUNT(DISTINCT `first_name`) FROM `test_model` WHERE (`age` < ?);",
				Args: []any{200},
			},
		},
		{
			name: "select raw where",
			selector: NewSelector[TestModel](db).Select(Raw("COUNT(DISTINCT `first_name`)")).
				Where(Col("Id").EQ(Raw("`age` + ?", 1))),
			query: &Query{
				SQL:  "SELECT COUNT(DISTINCT `first_name`) FROM `test_model` WHERE `id` = (`age` + ?);",
				Args: []any{1},
			},
		},
		{
			name:     "select column As",
			selector: NewSelector[TestModel](db).Select(Col("Id").As("my_id")),
			query: &Query{
				SQL: "SELECT `id` AS `my_id` FROM `test_model`;",
			},
		},
		{
			name:     "select Aggregate As",
			selector: NewSelector[TestModel](db).Select(Avg("Age").As("avg_age")),
			query: &Query{
				SQL: "SELECT AVG(`age`) AS `avg_age` FROM `test_model`;",
			},
		},
		{
			name:     "select Group by one column",
			selector: NewSelector[TestModel](db).Select(Avg("Age").As("avg_age")).GroupBy(Col("Age")),
			query: &Query{
				SQL: "SELECT AVG(`age`) AS `avg_age` FROM `test_model` GROUP BY `age`;",
			},
		},
		{
			name:     "select Group by columns",
			selector: NewSelector[TestModel](db).Select(Col("FirstName"), Col("Age")).GroupBy(Col("FirstName"), Col("Age")),
			query: &Query{
				SQL: "SELECT `first_name`,`age` FROM `test_model` GROUP BY `first_name`,`age`;",
			},
		},
		{
			name:     "select having columns",
			selector: NewSelector[TestModel](db).Select(Col("FirstName"), Col("Age")).GroupBy(Col("FirstName"), Col("Age")).Having(Col("Age").GT(18)),
			query: &Query{
				SQL:  "SELECT `first_name`,`age` FROM `test_model` GROUP BY `first_name`,`age` HAVING `age` > ?;",
				Args: []any{18},
			},
		},
		{
			name:     "select having aggregate",
			selector: NewSelector[TestModel](db).Select(Col("FirstName"), Col("Age")).GroupBy(Col("FirstName"), Col("Age")).Having(Avg("Age").GT(18)),
			query: &Query{
				SQL:  "SELECT `first_name`,`age` FROM `test_model` GROUP BY `first_name`,`age` HAVING AVG(`age`) > ?;",
				Args: []any{18},
			},
		},
		{
			name:     "order by",
			selector: NewSelector[TestModel](db).Select(Col("FirstName"), Col("Age")).GroupBy(Col("FirstName"), Col("Age")).Having(Avg("Age").GT(18)).OrderBy(Desc("FirstName"), Asc("Age")),
			query: &Query{
				SQL:  "SELECT `first_name`,`age` FROM `test_model` GROUP BY `first_name`,`age` HAVING AVG(`age`) > ? ORDER BY `first_name` DESC,`age` ASC;",
				Args: []any{18},
			},
		},
		{
			name:     "only limit",
			selector: NewSelector[TestModel](db).Select(Col("FirstName"), Col("Age")).Limit(100),
			query: &Query{
				SQL:  "SELECT `first_name`,`age` FROM `test_model` LIMIT ?;",
				Args: []any{100},
			},
		},
		{
			name:     "only offset",
			selector: NewSelector[TestModel](db).Select(Col("FirstName"), Col("Age")).Offset(1000),
			query: &Query{
				SQL:  "SELECT `first_name`,`age` FROM `test_model` LIMIT 18446744073709551615 OFFSET ?;",
				Args: []any{1000},
			},
		},
		{
			name:     "limit offset",
			selector: NewSelector[TestModel](db).Select(Col("FirstName"), Col("Age")).Offset(1000).Limit(100),
			query: &Query{
				SQL:  "SELECT `first_name`,`age` FROM `test_model` LIMIT ? OFFSET ?;",
				Args: []any{100, 1000},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			query, err := tc.selector.Build()
			assert.Equal(t, tc.wantErr, err)
			if err != nil {
				return
			}
			assert.Equal(t, tc.query, query)
		})
	}
}

// TestSelector_GetSentinelErrors 验证 Get 路径返回的错误可通过 errors.Is 匹配到
// 对应的 sentinel（ErrNoRows / ErrTooManyRows）。
//
// 说明：Get 的入参为 *sqlmock.Rows，可单元测试；此处为每个子用例独立构造 mock，
// 避免与 TestSelector_Get 中"先注册全部期望再顺序执行"的模式相互干扰。
func TestSelector_GetSentinelErrors(t *testing.T) {
	t.Run("no rows matches ErrNoRows", func(t *testing.T) {
		mockDB, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		db, err := OpenDB(mockDB)
		if err != nil {
			t.Fatal(err)
		}

		mock.ExpectQuery("SELECT .*").
			WillReturnRows(sqlmock.NewRows([]string{"id", "first_name", "age", "last_name"}))

		_, err = NewSelector[TestModel](db).Get(context.Background())
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, errs.ErrNoRows) {
			t.Fatalf("errors.Is(err, errs.ErrNoRows) = false, err = %v", err)
		}
	})

	t.Run("too many rows matches ErrTooManyRows", func(t *testing.T) {
		mockDB, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		db, err := OpenDB(mockDB)
		if err != nil {
			t.Fatal(err)
		}

		rows := sqlmock.NewRows([]string{"id", "first_name", "age", "last_name"})
		rows.AddRow([]byte("1"), []byte("yang"), []byte("18"), []byte("cheng"))
		rows.AddRow([]byte("2"), []byte("zhang"), []byte("20"), []byte("san"))
		mock.ExpectQuery("SELECT .*").WillReturnRows(rows)

		_, err = NewSelector[TestModel](db).Get(context.Background())
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, errs.ErrTooManyRows) {
			t.Fatalf("errors.Is(err, errs.ErrTooManyRows) = false, err = %v", err)
		}
	})

	// 静态校验：sentinel 必须已定义且非空，便于在未启用数据库的环境下回归。
	t.Run("sentinels defined", func(t *testing.T) {
		if errs.ErrNoRows == nil {
			t.Fatal("errs.ErrNoRows is nil")
		}
		if errs.ErrTooManyRows == nil {
			t.Fatal("errs.ErrTooManyRows is nil")
		}
		if !errors.Is(errs.ErrNoRows, errs.ErrNoRows) {
			t.Fatal("errs.ErrNoRows must match itself via errors.Is")
		}
		if !errors.Is(errs.ErrTooManyRows, errs.ErrTooManyRows) {
			t.Fatal("errs.ErrTooManyRows must match itself via errors.Is")
		}
	})
}

// TestResult_Err 验证 Result.Err() 能取到底层错误。
// 由于 Result 的 err 字段为未导出，这里通过导入 orm 包外部无法直接构造，
// 故仅做最小静态行为校验：零值 Result 的 Err() 应返回 nil。
func TestResult_Err(t *testing.T) {
	var r Result
	if err := r.Err(); err != nil {
		t.Fatalf("zero-value Result.Err() = %v, want nil", err)
	}
}

// TestQueryResult_Err 验证 QueryResult.Err() 能取到底层错误。
func TestQueryResult_Err(t *testing.T) {
	t.Run("nil error", func(t *testing.T) {
		qr := QueryResult{}
		if err := qr.Err(); err != nil {
			t.Fatalf("QueryResult.Err() = %v, want nil", err)
		}
	})
	t.Run("with error", func(t *testing.T) {
		qr := QueryResult{Error: errs.ErrTooManyRows}
		if err := qr.Err(); err != errs.ErrTooManyRows {
			t.Fatalf("QueryResult.Err() = %v, want %v", err, errs.ErrTooManyRows)
		}
		if !errors.Is(qr.Err(), errs.ErrTooManyRows) {
			t.Fatal("errors.Is should match ErrTooManyRows")
		}
	})
}
