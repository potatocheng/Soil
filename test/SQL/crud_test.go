package SQL

import (
	"context"
	"database/sql"
	_ "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestDB(t *testing.T) {
	db, err := sql.Open("mysql", "root:yyc167943@tcp(192.168.146.128:3306)/test?charset=utf8")
	require.NoError(t, err)
	defer func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)

	_, err = db.ExecContext(ctx, `
	CREATE TABLE IF NOT EXISTS test_model(
	id INTEGER PRIMARY KEY,
		first_name TEXT NOT NULL,
		age INTEGER,
		last_name TEXT NOT NULL
	)`)

	require.NoError(t, err)

	//res, err := db.ExecContext(ctx, "INSERT INTO test_model(`id`, `first_name`, `age`, `last_name`) VALUES(?, ?, ?, ?)",
	//	1, "Tom", 18, "Jerry")
	//require.NoError(t, err)
	//affected, err := res.RowsAffected()
	//log.Println("受影响行数: ", affected)
	//lastId, err := res.LastInsertId()
	//log.Println("最后插入 ID: ", lastId)

	row := db.QueryRowContext(ctx, "SELECT `id`, `first_name`, `age`, `first_name` FROM `test_model` WHERE `id` = ?", 1)
	require.NoError(t, row.Err())
	tm := &TestModel{}
	err = row.Scan(&tm.Id, &tm.FirstName, &tm.Age, &tm.LastName)
	require.Error(t, sql.ErrNoRows, err)

	cancel()
}

type TestModel struct {
	Id        int64
	FirstName string
	Age       int8
	LastName  *sql.NullString
}
