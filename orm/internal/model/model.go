package model

import (
	"reflect"
)

const (
	tagKeyColumn    = "column"
	tagKeyCreatedAt = "created_at"
	tagKeyUpdatedAt = "updated_at"
	tagKeyDeletedAt = "deleted_at"
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

	// CreatedAtField 自动维护的创建时间字段，nil 表示模型未定义该字段。
	// 字段名默认为 CreatedAt，可通过 orm:"created_at()" tag 标记其它字段。
	CreatedAtField *Field
	// UpdatedAtField 自动维护的更新时间字段，nil 表示模型未定义该字段。
	// 字段名默认为 UpdatedAt，可通过 orm:"updated_at()" tag 标记其它字段。
	UpdatedAtField *Field
	// DeletedAtField 软删除字段，nil 表示模型未定义该字段（执行物理删除）。
	// 字段名默认为 DeletedAt，可通过 orm:"deleted_at()" tag 标记其它字段。
	// 通常为 *time.Time 类型，NULL 表示未删除。
	DeletedAtField *Field
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
