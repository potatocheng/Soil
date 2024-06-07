package orm

import (
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestDeleter_Build(t *testing.T) {

	type TestModel struct {
		Id        int64
		FirstName string
		Age       uint8
		LastName  string
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
		name string
		del  QueryBuilder

		wantErr   error
		wantQuery *Query
	}{
		{
			name: "no from",
			del:  NewDeleter[TestModel](db),
			wantQuery: &Query{
				SQL: "DELETE FROM `test_model`",
			},
		},
		{
			name: "from",
			del:  NewDeleter[TestModel](db).From("test_model_t"),
			wantQuery: &Query{
				SQL: "DELETE FROM `test_model_t`",
			},
		},
		{
			name: "simple where",
			del: NewDeleter[TestModel](db).From("test_model_t").
				Where(Col("Id").EQ(12)),
			wantQuery: &Query{
				SQL:  "DELETE FROM `test_model_t` WHERE `id` = ?;",
				Args: []any{12},
			},
		},
		{
			name: "传入参数是predicate切片的where",
			del: NewDeleter[TestModel](db).From("test_model_t").
				Where(Col("Id").EQ(12), Col("LastName").EQ("yi")),
			wantQuery: &Query{
				SQL:  "DELETE FROM `test_model_t` WHERE `id` = ? AND `last_name` = ?;",
				Args: []any{12, "yi"},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			q, err := testCase.del.Build()
			assert.Equal(t, testCase.wantErr, err)
			if err != nil {
				return
			}

			assert.Equal(t, q, q)
		})
	}
}
