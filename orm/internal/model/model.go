package model

import (
	"reflect"
)

const (
	tagKeyColumn = "column"
)

type ModelOpt func(model *Model) error

// Model 一个model对应一个数据表
type Model struct {
	TableName string
	// FieldMap key: go结构体中字段名称
	FieldMap map[string]*Field
	// ColumnMap key: 数据库中列名, 主要是为了提高查询速度
	ColumnMap map[string]*Field
	// Fields :为了记录struct field的顺序，目前主要使用在Insert中
	Fields []*Field
}

// Field 列的属性，比如列名，是否是主键...
type Field struct {
	ColName string
	GoName  string
	Type    reflect.Type
	Offset  uintptr
}

// TableName 自定义表明
type TableName interface {
	TableName() string
}
