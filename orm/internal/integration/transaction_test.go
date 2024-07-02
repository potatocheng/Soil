//go:build e2e

package integration

import (
	"Soil/orm"
	"context"
	_ "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestTransaction(t *testing.T) {
	// 创建DB
	db, err := orm.Open("mysql", "root:root@tcp(localhost:13306)/integration_test?charset=utf8")
	require.NoError(t, err)
	testCases := []struct {
		name       string
		cs         map[string][]byte
		fn         func()
		wantResult *SimpleStruct
	}{
		{
			name: "transaction",
			fn: func() {
				tx, err := db.BeginTx(context.Background(), nil)
				require.NoError(t, err)
				result := orm.NewInserter[SimpleStruct](tx).Values(NewSimpleStruct(1)).Exec(context.Background())
				err = tx.Commit() // 记住commit要不然会产生悬挂事务问题，所以可以使用DoTx来管理事务,见下一个例子
				require.NoError(t, err)
				_, err = result.RowsAffected()
				require.NoError(t, err)
			},
			wantResult: NewSimpleStruct(1),
		},
		{
			name: "doTx -- auto manage transaction",
			fn: func() {
				err = db.DoTx(context.Background(), func(ctx context.Context, tx *orm.Tx) error {
					result := orm.NewInserter[SimpleStruct](tx).Values(NewSimpleStruct(2)).Exec(context.Background())
					_, err = result.RowsAffected()
					return err
				}, nil)
			},
			wantResult: NewSimpleStruct(2),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			testCase.fn()
			data, err := orm.NewSelector[SimpleStruct](db).Where(orm.Col("Id").EQ(testCase.wantResult.Id)).Get(context.Background())
			require.NoError(t, err)
			assert.Equal(t, data, testCase.wantResult)
		})
	}
}
