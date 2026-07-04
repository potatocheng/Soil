package errs

import (
	"errors"
	"fmt"
)

var (
	ErrEntityNil              = errors.New("orm: 传入的entity为空")
	ErrPointerOnly            = errors.New("orm: 只支持一级指针作为输入，例如*User")
	ErrNoRows                 = errors.New("orm: 未找到数据")
	ErrTooManyRows            = errors.New("orm: 查询到过多数据")
	ErrTooManyReturnedColumns = errors.New("orm: 查询返回的列数比接收的列数多")
	ErrInsertZeroRow          = errors.New("orm: 插入0行")
	ErrInvalidColumn          = errors.New("orm: 非法列")
	ErrUnsupportedFeature     = errors.New("orm: 不支持的功能")
	ErrNilPointer             = errors.New("orm: 空指针")
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

// NewErrInvalidColumn 返回一个 wrap 了 ErrInvalidColumn sentinel 的错误，
// 调用方可以使用 errors.Is(err, ErrInvalidColumn) 进行匹配。
func NewErrInvalidColumn(name string) error {
	return fmt.Errorf("%w: %s", ErrInvalidColumn, name)
}

func NewErrUnsupportedAssignableType(exp any) error {
	return fmt.Errorf("orm: 不支持的 Assignable 表达式 %v", exp)
}

func NewErrFailToRollbackTx(bizErr error, rbErr error, panicked bool) error {
	return fmt.Errorf("orm: 回滚事务失败，业务错误 %w, 回滚错误 %s, panic: %t", bizErr, rbErr.Error(), panicked)
}

func NewErrUnsupportedTable(table any) error {
	return fmt.Errorf("orm: 不支持TableReference类型 %v", table)
}

func NewErrUnsupportedFeature(feature string) error {
	return fmt.Errorf("%w: %s", ErrUnsupportedFeature, feature)
}
