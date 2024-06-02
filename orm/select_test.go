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
			res, err := NewSelector[TestModel](db).GetV1(context.Background())
			assert.Equal(t, err, testCase.wantErr)
			if err != nil {
				return
			}
			assert.Equal(t, testCase.wantVal, res)
		})
	}
}
