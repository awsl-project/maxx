package sqlite

import (
	"log"
	"sort"
	"time"

	"gorm.io/gorm"
)

// Migration 表示一个数据库迁移
type Migration struct {
	Version     int
	Description string
	Up          func(db *gorm.DB) error
	Down        func(db *gorm.DB) error
}

// 所有迁移按版本号注册
// 注意：GORM AutoMigrate 会自动处理新增列，这里只需要处理特殊情况（重命名、数据迁移等）
var migrations = []Migration{
	{
		Version:     1,
		Description: "Convert cost from microUSD to nanoUSD (multiply by 1000)",
		Up: func(db *gorm.DB) error {
			// Convert cost in proxy_requests table
			if err := db.Exec("UPDATE proxy_requests SET cost = cost * 1000 WHERE cost > 0").Error; err != nil {
				return err
			}
			// Convert cost in proxy_upstream_attempts table
			if err := db.Exec("UPDATE proxy_upstream_attempts SET cost = cost * 1000 WHERE cost > 0").Error; err != nil {
				return err
			}
			// Convert cost in usage_stats table
			if err := db.Exec("UPDATE usage_stats SET cost = cost * 1000 WHERE cost > 0").Error; err != nil {
				return err
			}
			return nil
		},
		Down: func(db *gorm.DB) error {
			// Rollback: divide by 1000
			if err := db.Exec("UPDATE proxy_requests SET cost = cost / 1000").Error; err != nil {
				return err
			}
			if err := db.Exec("UPDATE proxy_upstream_attempts SET cost = cost / 1000").Error; err != nil {
				return err
			}
			if err := db.Exec("UPDATE usage_stats SET cost = cost / 1000").Error; err != nil {
				return err
			}
			return nil
		},
	},
}

// RunMigrations 运行所有待执行的迁移
func (d *DB) RunMigrations() error {
	// 确保迁移表存在（由 GORM AutoMigrate 处理）
	if err := d.gorm.AutoMigrate(&SchemaMigration{}); err != nil {
		return err
	}

	// 如果没有迁移，直接返回
	if len(migrations) == 0 {
		return nil
	}

	// 获取当前版本
	currentVersion := d.getCurrentVersion()

	// 按版本号排序迁移
	sortedMigrations := make([]Migration, len(migrations))
	copy(sortedMigrations, migrations)
	sort.Slice(sortedMigrations, func(i, j int) bool {
		return sortedMigrations[i].Version < sortedMigrations[j].Version
	})

	// 运行所有版本大于当前版本的迁移
	for _, m := range sortedMigrations {
		if m.Version <= currentVersion {
			continue
		}

		log.Printf("[Migration] Running migration v%d: %s", m.Version, m.Description)

		if err := d.runMigration(m); err != nil {
			log.Printf("[Migration] Failed migration v%d: %v", m.Version, err)
			return err
		}

		log.Printf("[Migration] Completed migration v%d", m.Version)
	}

	return nil
}

// getCurrentVersion 获取当前数据库版本
func (d *DB) getCurrentVersion() int {
	var maxVersion int
	d.gorm.Model(&SchemaMigration{}).Select("COALESCE(MAX(version), 0)").Scan(&maxVersion)
	return maxVersion
}

// runMigration 在事务中运行单个迁移
func (d *DB) runMigration(m Migration) error {
	return d.gorm.Transaction(func(tx *gorm.DB) error {
		// 运行迁移
		if m.Up != nil {
			if err := m.Up(tx); err != nil {
				return err
			}
		}

		// 记录迁移
		return tx.Create(&SchemaMigration{
			Version:     m.Version,
			Description: m.Description,
			AppliedAt:   time.Now().UnixMilli(),
		}).Error
	})
}

// RollbackMigration 回滚到指定版本
func (d *DB) RollbackMigration(targetVersion int) error {
	currentVersion := d.getCurrentVersion()

	if targetVersion >= currentVersion {
		log.Printf("[Migration] Already at version %d, target is %d, nothing to rollback", currentVersion, targetVersion)
		return nil
	}

	// 按版本号降序排序
	sortedMigrations := make([]Migration, len(migrations))
	copy(sortedMigrations, migrations)
	sort.Slice(sortedMigrations, func(i, j int) bool {
		return sortedMigrations[i].Version > sortedMigrations[j].Version
	})

	// 回滚所有版本大于目标版本的迁移
	for _, m := range sortedMigrations {
		if m.Version <= targetVersion {
			break
		}
		if m.Version > currentVersion {
			continue
		}

		log.Printf("[Migration] Rolling back migration v%d: %s", m.Version, m.Description)

		if err := d.rollbackMigration(m); err != nil {
			log.Printf("[Migration] Failed rollback v%d: %v", m.Version, err)
			return err
		}

		log.Printf("[Migration] Rolled back migration v%d", m.Version)
	}

	return nil
}

// rollbackMigration 在事务中回滚单个迁移
func (d *DB) rollbackMigration(m Migration) error {
	return d.gorm.Transaction(func(tx *gorm.DB) error {
		// 运行回滚
		if m.Down != nil {
			if err := m.Down(tx); err != nil {
				return err
			}
		}

		// 删除迁移记录
		return tx.Where("version = ?", m.Version).Delete(&SchemaMigration{}).Error
	})
}

// GetMigrationStatus 获取迁移状态
func (d *DB) GetMigrationStatus() ([]MigrationStatus, error) {
	// 获取已应用的迁移
	var applied []SchemaMigration
	if err := d.gorm.Find(&applied).Error; err != nil {
		return nil, err
	}

	appliedMap := make(map[int]int64)
	for _, m := range applied {
		appliedMap[m.Version] = m.AppliedAt
	}

	// 构建状态列表
	var statuses []MigrationStatus
	for _, m := range migrations {
		status := MigrationStatus{
			Version:     m.Version,
			Description: m.Description,
			Applied:     false,
		}
		if appliedAt, ok := appliedMap[m.Version]; ok {
			status.Applied = true
			status.AppliedAt = fromTimestamp(appliedAt)
		}
		statuses = append(statuses, status)
	}

	// 按版本号排序
	sort.Slice(statuses, func(i, j int) bool {
		return statuses[i].Version < statuses[j].Version
	})

	return statuses, nil
}

// MigrationStatus 迁移状态
type MigrationStatus struct {
	Version     int
	Description string
	Applied     bool
	AppliedAt   time.Time
}
