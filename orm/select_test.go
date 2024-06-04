package orm

import (
	"Soil/orm/internal/errs"
	"context"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSelector_Build(t *testing.T) {
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
			selector: NewSelector[TestModel](db).From("`test_model_t`"),
			wantQuery: &Query{
				SQL: "SELECT * FROM `test_model_t`;",
			},
		},
		{
			name:     "empty from",
			selector: NewSelector[TestModel](db).From(""),
			wantQuery: &Query{
				SQL: "SELECT * FROM `test_model`;",
			},
		},
		{
			name:     "with db",
			selector: NewSelector[TestModel](db).From("`test_db`.`test_model`"),
			wantQuery: &Query{
				SQL: "SELECT * FROM `test_db`.`test_model`;",
			},
		},
		{
			name: "single and simple predicate",
			selector: NewSelector[TestModel](db).From("`test_db`.`test_model`").
				Where(Col("Age").EQ(18)),
			wantQuery: &Query{
				SQL:  "SELECT * FROM `test_db`.`test_model` WHERE `age` = ?;",
				Args: []any{18},
			},
		},
		{
			name: "or",
			selector: NewSelector[TestModel](db).From("`test_db`.`test_model`").
				Where(Col("Age").EQ(18).Or(Col("Id").EQ(1))),
			wantQuery: &Query{
				SQL:  "SELECT * FROM `test_db`.`test_model` WHERE (`age` = ?) OR (`id` = ?);",
				Args: []any{18, 1},
			},
		},
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
