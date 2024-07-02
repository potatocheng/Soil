//go:build e2e

package integration

import (
	"Soil/orm"
	"context"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"testing"
)

type InsertSuite struct {
	suite.Suite
	driver string
	dsn    string

	db *orm.DB
}

func TestMySQLInsert(t *testing.T) {
	suite.Run(t, &InsertSuite{
		driver: "mysql",
		dsn:    "root:root@tcp(localhost:13306)/integration_test?charset=utf8",
	})
}

// SetupSuite 在运行测试前执行
func (i *InsertSuite) SetupSuite() {
	db, err := orm.Open(i.driver, i.dsn)
	require.NoError(i.T(), err)
	i.db = db
}

func (i *InsertSuite) TestInsert() {
	db := i.db
	testCases := []struct {
		name         string
		inserter     *orm.Inserter[SimpleStruct]
		wantAffected int64
		wantResults  []*SimpleStruct
	}{
		{
			name:         "insert 1 row",
			inserter:     orm.NewInserter[SimpleStruct](db).Values(NewSimpleStruct(1)),
			wantAffected: 1,
			wantResults:  []*SimpleStruct{NewSimpleStruct(1)},
		},
		{
			name:         "insert multiple rows",
			inserter:     orm.NewInserter[SimpleStruct](db).Values(NewSimpleStruct(2), NewSimpleStruct(3)),
			wantAffected: 2,
			wantResults:  []*SimpleStruct{NewSimpleStruct(2), NewSimpleStruct(3)},
		},
		{
			name:         "upsert",
			inserter:     orm.NewInserter[SimpleStruct](db).Values(NewSimpleStruct(3)).OnDuplicateKey().Update(orm.Assign("Int16", 200)),
			wantAffected: 2,
		},
	}

	for _, tc := range testCases {
		i.T().Run(tc.name, func(t *testing.T) {
			result := tc.inserter.Exec(context.Background())
			affected, err := result.RowsAffected()
			require.NoError(i.T(), err)
			assert.Equal(i.T(), tc.wantAffected, affected)

			for _, wantResult := range tc.wantResults {
				selectResult, err := orm.NewSelector[SimpleStruct](db).Where(orm.Col("Id").EQ(wantResult.Id)).Get(context.Background())
				assert.NoError(i.T(), err)
				assert.Equal(i.T(), wantResult, selectResult)
			}
		})
	}
}
