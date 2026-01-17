# 数据库迁移指南

本文档介绍项目的数据库迁移机制和最佳实践。

## 概述

项目使用 **GORM** 作为 ORM，采用两层迁移策略：

1. **AutoMigrate**：自动处理新增表和列
2. **自定义迁移**：处理特殊情况（重命名、数据迁移等）

## 工作原理

### 启动流程

```
应用启动
    ↓
db.Initialize()
    ↓
AutoMigrate(所有模型)  ← 自动创建/添加新表和列
    ↓
RunMigrations()        ← 执行自定义迁移
    ↓
应用就绪
```

### AutoMigrate 能力

GORM 的 AutoMigrate 会自动：
- ✅ 创建新表
- ✅ 添加新列
- ✅ 创建索引

AutoMigrate **不会**：
- ❌ 删除列
- ❌ 重命名列
- ❌ 修改列类型
- ❌ 数据迁移

这些操作需要通过自定义迁移处理。

## 添加新迁移

### 文件位置

`internal/repository/sqlite/migrations.go`

### 迁移结构

```go
type Migration struct {
    Version     int
    Description string
    Up          func(db *gorm.DB) error
    Down        func(db *gorm.DB) error
}
```

### 示例：重命名列

```go
var migrations = []Migration{
    {
        Version:     1,
        Description: "rename old_column to new_column in users table",
        Up: func(db *gorm.DB) error {
            if db.Migrator().HasColumn(&User{}, "old_column") {
                return db.Migrator().RenameColumn(&User{}, "old_column", "new_column")
            }
            return nil
        },
        Down: func(db *gorm.DB) error {
            if db.Migrator().HasColumn(&User{}, "new_column") {
                return db.Migrator().RenameColumn(&User{}, "new_column", "old_column")
            }
            return nil
        },
    },
}
```

### 示例：删除列

```go
{
    Version:     2,
    Description: "remove deprecated_field from orders",
    Up: func(db *gorm.DB) error {
        if db.Migrator().HasColumn(&Order{}, "deprecated_field") {
            return db.Migrator().DropColumn(&Order{}, "deprecated_field")
        }
        return nil
    },
    Down: func(db *gorm.DB) error {
        // 如果需要回滚，添加回列
        return db.Migrator().AddColumn(&Order{}, "deprecated_field")
    },
}
```

### 示例：数据迁移

```go
{
    Version:     3,
    Description: "migrate data from old format to new format",
    Up: func(db *gorm.DB) error {
        // 使用事务保证数据一致性
        return db.Exec(`
            UPDATE users
            SET full_name = first_name || ' ' || last_name
            WHERE full_name IS NULL OR full_name = ''
        `).Error
    },
    Down: func(db *gorm.DB) error {
        // 数据迁移通常不可逆
        return nil
    },
}
```

## GORM Migrator API

常用方法：

| 方法 | 说明 |
|------|------|
| `HasTable(&Model{})` | 检查表是否存在 |
| `HasColumn(&Model{}, "column")` | 检查列是否存在 |
| `AddColumn(&Model{}, "column")` | 添加列 |
| `DropColumn(&Model{}, "column")` | 删除列 |
| `RenameColumn(&Model{}, "old", "new")` | 重命名列 |
| `AlterColumn(&Model{}, "column")` | 修改列类型 |
| `HasIndex(&Model{}, "index_name")` | 检查索引是否存在 |
| `CreateIndex(&Model{}, "index_name")` | 创建索引 |
| `DropIndex(&Model{}, "index_name")` | 删除索引 |
| `RenameIndex(&Model{}, "old", "new")` | 重命名索引 |

## 最佳实践

### 1. 版本号递增

每个迁移必须有唯一的递增版本号：

```go
var migrations = []Migration{
    {Version: 1, ...},
    {Version: 2, ...},
    {Version: 3, ...},
}
```

### 2. 幂等性检查

迁移应该是幂等的，重复执行不会出错：

```go
Up: func(db *gorm.DB) error {
    // ✅ 先检查再操作
    if db.Migrator().HasColumn(&User{}, "old_column") {
        return db.Migrator().RenameColumn(&User{}, "old_column", "new_column")
    }
    return nil
},
```

### 3. 描述清晰

描述应该清楚说明迁移的目的：

```go
// ✅ 好的描述
Description: "rename user.email_address to user.email"

// ❌ 不好的描述
Description: "update users"
```

### 4. 提供 Down 方法

尽量提供回滚方法，方便开发调试：

```go
Down: func(db *gorm.DB) error {
    // 回滚逻辑
    return db.Migrator().RenameColumn(&User{}, "email", "email_address")
},
```

### 5. 使用事务

复杂迁移应该使用事务：

```go
Up: func(db *gorm.DB) error {
    return db.Transaction(func(tx *gorm.DB) error {
        if err := tx.Exec("UPDATE ...").Error; err != nil {
            return err
        }
        if err := tx.Exec("DELETE ...").Error; err != nil {
            return err
        }
        return nil
    })
},
```

## GORM 模型注意事项

### 列名映射

GORM 默认将 CamelCase 转换为 snake_case，但对于包含数字的字段需要特别注意：

```go
// ❌ 默认会映射到 cache5m_write_count（没有下划线）
Cache5mWriteCount uint64

// ✅ 使用 column tag 指定正确的列名
Cache5mWriteCount uint64 `gorm:"column:cache_5m_write_count"`
```

### 时间字段

项目使用 Unix 毫秒时间戳存储时间：

```go
type BaseModel struct {
    ID        uint64 `gorm:"primaryKey;autoIncrement"`
    CreatedAt int64  `gorm:"not null"`  // Unix 毫秒时间戳
    UpdatedAt int64  `gorm:"not null"`  // Unix 毫秒时间戳
}
```

## 迁移状态

迁移记录存储在 `schema_migrations` 表：

```sql
SELECT * FROM schema_migrations;
```

| version | description | applied_at |
|---------|-------------|------------|
| 1 | rename old_column to new_column | 1705500000000 |
| 2 | remove deprecated_field | 1705600000000 |

## 回滚迁移

目前支持通过代码回滚到指定版本：

```go
// 回滚到版本 1
db.RollbackMigration(1)
```

## 常见问题

### Q: 新增字段需要写迁移吗？

**A**: 不需要。只需要在 GORM 模型中添加字段，AutoMigrate 会自动创建新列。

### Q: 什么时候需要写迁移？

**A**: 以下情况需要自定义迁移：
- 重命名列
- 删除列
- 修改列类型
- 数据迁移/转换
- 复杂的索引操作

### Q: 迁移失败怎么办？

**A**: 迁移在事务中执行，失败会自动回滚。修复迁移代码后重新启动即可。

### Q: 如何测试迁移？

**A**:
1. 备份数据库
2. 在测试环境运行迁移
3. 验证数据完整性
4. 测试回滚功能
