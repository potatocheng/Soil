package errs

import (
	"errors"
	"fmt"
)

var (
	ErrEntityNil   = errors.New("orm: 传入的entity为空")
	ErrPointerOnly = errors.New("orm: 只支持一级指针作为输入，例如*User")
	//ErrTagNotMatch = errors.New("orm: 没有找到key为orm的标签")
	ErrNoRows                 = errors.New("orm: 未找到数据")
	ErrTooManyReturnedColumns = errors.New("orm: 查询返回的列数比接收的列数多")
)

func NewErrUnsupportedExpressionType(exp any) error {
	return fmt.Errorf("orm: 不支持表达式 %v", exp)
}

func NewErrInvalidTagContent(pair string) error {
	return fmt.Errorf("orm: 非法标签值 %s", pair)
}

func NewErrUnknownField(field string) error {
	return fmt.Errorf("orm: 未知字段 %s", field)
}

func NewErrUnknownColumn(column string) error {
	return fmt.Errorf("orm: 未知列 %s", column)
}
