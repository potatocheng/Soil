package querylog

import (
	"Soil/orm"
	"context"
	"database/sql"
	_ "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

type TestModel struct {
	Id        int64
	FirstName string
	Age       int8
	LastName  *sql.NullString
}

func TestQueryLog(t *testing.T) {
	var wantQuery string
	var wantArgs []any
	m := (&MiddlewareBuilder{}).WithLogFunc(func(sql string, args []any) {
		wantQuery = sql
		wantArgs = args
	})

	db, err := orm.Open("mysql",
		"root:root@tcp(localhost:13306)/integration_test?charset=utf8",
		orm.DBWithMiddlewares(m.Build()))
	require.NoError(t, err)
	_, _ = orm.NewSelector[TestModel](db).Where(orm.Col("Id").EQ(1)).Get(context.Background())
	assert.Equal(t, "SELECT * FROM `test_model` WHERE `id` = ?;", wantQuery)
	assert.Equal(t, []any{1}, wantArgs)

	// 没有连接数据库，忽略返回值
	_ = orm.NewUpdater[TestModel](db).Update(&TestModel{
		Id:        int64(1),
		FirstName: "Jay",
		Age:       int8(2),
	}).Set(orm.Col("Id"), orm.Col("FirstName"), orm.Col("Age")).Where(orm.Col("Age").GT(18)).Exec(context.Background())
	assert.Equal(t, "UPDATE `test_model` SET `id`=?,`first_name`=?,`age`=? WHERE `age` > ?;", wantQuery)
	assert.Equal(t, []any{int64(1), "Jay", int8(2), 18}, wantArgs)

	_ = orm.NewDeleter[TestModel](db).Where(orm.Col("Id").GT(18)).Exec(context.Background())
	assert.Equal(t, "DELETE FROM `test_model` WHERE `id` > ?;", wantQuery)
	assert.Equal(t, []any{18}, wantArgs)
}
