package main

import (
	"fmt"
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

	// 获取 QuerySeter 对象，并设置表名orders
	qs := o.QueryTable("users")

	// 定义保存查询结果的变量
	var users []User

	// 使用QuerySeter 对象构造查询条件，并执行查询。
	num, err := qs.Filter("city", "shenzhen"). // 设置查询条件
							Filter("init_time__gt", "2019-06-28 22:00:00"). // 设置查询条件
							GroupBy("Id + Age").
							Limit(10).                    // 限制返回行数
							All(&users, "id", "username") // All 执行查询，并且返回结果，这里指定返回id和username字段，结果保存在users变量

	if err != nil {
		panic(err)
	}
	fmt.Println("结果行数:", num)
}
