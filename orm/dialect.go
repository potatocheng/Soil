package orm

import "Soil/orm/internal/errs"

var (
	MySQL  Dialect = &mysqlDialect{}
	SQLite Dialect = &sqliteDialect{}
)

type Dialect interface {
	quoter() byte
	// buildOnDuplicateKey 生成upsert语句都在这里处理
	buildUpsert(b *builder, upsert *Upsert) error
}

type standardSql struct{}

// buildOnDuplicateKey 在标准SQL中，UPSERT语义没有一个正式的标准化语法
func (s standardSql) buildUpsert(b *builder, upsert *Upsert) error {
	panic("implement me")
}

// quoter 标准SQL（SQL标准）规定的引用标识符（quote identifier）是双引号（""）
func (s standardSql) quoter() byte {
	return byte('"')
}

type mysqlDialect struct {
	standardSql
}

func (m mysqlDialect) quoter() byte {
	return '`'
}

func (m mysqlDialect) buildUpsert(b *builder, upsert *Upsert) error {
	b.sqlStrBuilder.WriteString(" ON DUPLICATE KEY UPDATE ")
	for idx, assign := range upsert.assigns {
		if idx != 0 {
			b.sqlStrBuilder.WriteByte(',')
		}
		switch assign := assign.(type) {
		case Assignment:
			return b.buildAssignment(assign)
		case Column:
			field, ok := b.model.FieldMap[assign.name]
			if !ok {
				return errs.NewErrUnknownField(assign.name)
			}
			b.quote(field.ColName)
			b.sqlStrBuilder.WriteString("=VALUES(")
			b.quote(field.ColName)
			b.sqlStrBuilder.WriteString(")")
		default:
			return errs.NewErrUnsupportedAssignableType(assign)
		}
	}

	return nil
}

type sqliteDialect struct {
	standardSql
}

func (s sqliteDialect) buildUpsert(b *builder, upsert *Upsert) error {
	b.sqlStrBuilder.WriteString(" ON CONFLICT(")
	for idx, conflictCol := range upsert.conflictColumns {
		if idx != 0 {
			b.sqlStrBuilder.WriteByte(',')
		}
		err := b.buildColumn(Col(conflictCol))
		if err != nil {
			return err
		}
	}
	b.sqlStrBuilder.WriteString(") DO UPDATE SET ")

	for idx, assign := range upsert.assigns {
		if idx != 0 {
			b.sqlStrBuilder.WriteByte(',')
		}
		switch assign := assign.(type) {
		case Assignment:
			return b.buildAssignment(assign)
		case Column:
			field, ok := b.model.FieldMap[assign.name]
			if !ok {
				return errs.NewErrUnknownField(assign.name)
			}
			b.quote(field.ColName)
			b.sqlStrBuilder.WriteString("=excluded.")
			b.quote(field.ColName)
		default:
			return errs.NewErrUnsupportedAssignableType(assign)
		}
	}

	return nil
}

// quoter 标准SQL（SQL标准）规定的引用标识符（quote identifier）是双引号（""）
func (s sqliteDialect) quoter() byte {
	return byte('`')
}
