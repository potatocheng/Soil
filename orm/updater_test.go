package orm

import (
	"fmt"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestUpdater(t *testing.T) {
	mockDB, _, err := sqlmock.New()
	require.NoError(t, err)
	db, err := OpenDB(mockDB)
	require.NoError(t, err)

	testCases := []struct {
		name       string
		updater    QueryBuilder
		wantErr    error
		wantResult *Query
	}{
		{
			name:    "simple update set assign",
			updater: NewUpdater[TestModel](db).Set(Assign("FirstName", "Jay")),

			wantResult: &Query{
				SQL:  "UPDATE `test_model` SET `first_name`=?;",
				Args: []any{"Jay"},
			},
		},
		{
			name:    "simple update set assign",
			updater: NewUpdater[TestModel](db).Set(Assign("FirstName", "Jay")),

			wantResult: &Query{
				SQL:  "UPDATE `test_model` SET `first_name`=?;",
				Args: []any{"Jay"},
			},
		},
		{
			name:    "simple update set assign",
			updater: NewUpdater[TestModel](db).Set(Assign("FirstName", "Jay")),

			wantResult: &Query{
				SQL:  "UPDATE `test_model` SET `first_name`=?;",
				Args: []any{"Jay"},
			},
		},
		{
			name: "simple update set assign",
			updater: NewUpdater[TestModel](db).Update(&TestModel{
				Id:        int64(1),
				FirstName: "Jay",
				Age:       uint8(2),
				LastName:  "Chou",
			}),

			wantResult: &Query{
				SQL:  "UPDATE `test_model` SET `id`=?,`first_name`=?,`age`=?,`last_name`=?;",
				Args: []any{int64(1), "Jay", uint8(2), "Chou"},
			},
		},
		{
			name: "simple update set assign",
			updater: NewUpdater[TestModel](db).Update(&TestModel{
				Id:        int64(1),
				FirstName: "Jay",
				Age:       uint8(2),
			}),
			wantErr: fmt.Errorf("orm: %s 没有设置值(可以使用Set指定设置了值待修改的列)", "LastName"),
		},
		{
			name: "update by column",
			updater: NewUpdater[TestModel](db).Update(&TestModel{
				Id:        int64(1),
				FirstName: "Jay",
				Age:       uint8(2),
			}).Set(Col("Id"), Col("FirstName"), Col("Age")),
			wantResult: &Query{
				SQL:  "UPDATE `test_model` SET `id`=?,`first_name`=?,`age`=?;",
				Args: []any{int64(1), "Jay", uint8(2)},
			},
		},
		{
			name: "update where",
			updater: NewUpdater[TestModel](db).Update(&TestModel{
				Id:        int64(1),
				FirstName: "Jay",
				Age:       uint8(2),
			}).Set(Col("Id"), Col("FirstName"), Col("Age")).Where(Col("Age").GT(18)),
			wantResult: &Query{
				SQL:  "UPDATE `test_model` SET `id`=?,`first_name`=?,`age`=? WHERE `age` > ?;",
				Args: []any{int64(1), "Jay", uint8(2), 18},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			q, err := tc.updater.Build()
			assert.Equal(t, err, tc.wantErr)
			if err != nil {
				return
			}
			assert.Equal(t, q, tc.wantResult)
		})
	}
}
