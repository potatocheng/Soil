package main

import (
	"context"
	"github.com/beego/beego/v2/client/orm"

	_ "github.com/go-sql-driver/mysql"
)

type User struct {
	Id   int
	Name string
	Age  int
}

func init() {
	// 设置数据库
	orm.RegisterDataBase("default", "mysql", "user:password@/dbname?charset=utf8")

	// 注册模型
	orm.RegisterModel(new(User))

	// 创建表
	orm.RunSyncdb("default", false, true)
}

func main() {
	o := orm.NewOrm()
	o.BeginWithCtx(context.Background())
}
