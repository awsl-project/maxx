package gormdb

import (
	"database/sql/driver"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// LongText 是一个跨数据库兼容的长文本类型
// - MySQL: LONGTEXT (最大 4GB)
// - PostgreSQL: TEXT (无限制)
// - SQLite: TEXT (无限制)
type LongText string

// GormDBDataType 实现 GormDBDataTypeInterface 接口
// 根据不同的数据库返回合适的类型
func (LongText) GormDBDataType(db *gorm.DB, field *schema.Field) string {
	switch db.Dialector.Name() {
	case "mysql":
		return "LONGTEXT"
	case "postgres":
		return "TEXT"
	case "sqlite":
		return "TEXT"
	default:
		return "TEXT"
	}
}

// Value 实现 driver.Valuer 接口，用于将 LongText 转换为数据库值
func (lt LongText) Value() (driver.Value, error) {
	return string(lt), nil
}

// Scan 实现 sql.Scanner 接口，用于从数据库值转换为 LongText
func (lt *LongText) Scan(value interface{}) error {
	if value == nil {
		*lt = ""
		return nil
	}

	switch v := value.(type) {
	case string:
		*lt = LongText(v)
		return nil
	case []byte:
		*lt = LongText(v)
		return nil
	default:
		return fmt.Errorf("unsupported LongText scan type %T", value)
	}
}

// String 返回字符串值
func (lt LongText) String() string {
	return string(lt)
}
