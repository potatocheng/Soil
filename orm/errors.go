package orm

import "Soil/orm/internal/errs"

// 此文件将 internal/errs 中的 sentinel 错误在 orm 包级别重新导出，
// 便于使用方通过 orm.ErrXxx 直接引用，并支持 errors.Is 匹配。
var (
	ErrOptimisticLock = errs.ErrOptimisticLock
)
