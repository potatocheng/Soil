package orm

import "context"

// 钩子接口：模型（通常以指针接收者）实现这些方法时，orm 会在对应操作的前后自动调用。
// 钩子返回 error 时将中止当前操作并把该 error 返回给调用方。
// 钩子接收的 ctx 与 Exec/Get 接收的 ctx 一致，可透传。
//
// 调用时机：
//   - Inserter.Exec:  BeforeInsert 在生成/执行 SQL 前调用（钩子内对模型的修改会反映到生成的 SQL）；
//                     AfterInsert  在 SQL 执行成功后调用。
//   - Selector.Get:   BeforeQuery 在生成/执行 SQL 前调用；
//                     AfterQuery  在结果填充成功后调用。
//   - Updater.Exec:   仅当 Updater 通过 Update(val) 持有模型实例时调用 BeforeUpdate/AfterUpdate；
//                     否则跳过（例如仅使用 Set(...) 的批量更新场景）。
//
// Deleter 当前不实现钩子：Deleter 只持有表名与 WHERE 条件，没有可识别的模型实例引用，
// 无法在其上调用模型方法风格的钩子。如需删除前/后逻辑，请使用中间件或在业务层显式调用。
// 后续若为 Deleter 增加模型实例引用，可再补充 BeforeDelete/AfterDelete 支持。

// BeforeInsert 在 INSERT 执行前调用。
type BeforeInsert interface {
	BeforeInsert(ctx context.Context) error
}

// AfterInsert 在 INSERT 执行成功后调用。
type AfterInsert interface {
	AfterInsert(ctx context.Context) error
}

// BeforeUpdate 在 UPDATE 执行前调用（仅 Updater 持有模型实例时）。
type BeforeUpdate interface {
	BeforeUpdate(ctx context.Context) error
}

// AfterUpdate 在 UPDATE 执行成功后调用（仅 Updater 持有模型实例时）。
type AfterUpdate interface {
	AfterUpdate(ctx context.Context) error
}

// BeforeDelete 在 DELETE 执行前调用（当前 Deleter 未实现，预留接口）。
type BeforeDelete interface {
	BeforeDelete(ctx context.Context) error
}

// AfterDelete 在 DELETE 执行成功后调用（当前 Deleter 未实现，预留接口）。
type AfterDelete interface {
	AfterDelete(ctx context.Context) error
}

// BeforeQuery 在 SELECT 执行前调用。
type BeforeQuery interface {
	BeforeQuery(ctx context.Context) error
}

// AfterQuery 在 SELECT 结果填充成功后调用。
type AfterQuery interface {
	AfterQuery(ctx context.Context) error
}
