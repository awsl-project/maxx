package sqlite

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

// Migration 表示一个数据库迁移
type Migration struct {
	Version     int
	Description string
	Up          func(db *gorm.DB) error
	Down        func(db *gorm.DB) error
}

// tenantScopedTables lists all tables that need tenant_id populated during migration v3
var tenantScopedTables = []string{
	"providers", "projects", "routes", "sessions",
	"retry_configs", "routing_strategies", "api_tokens", "model_mappings",
	"proxy_requests", "proxy_upstream_attempts", "usage_stats",
	"antigravity_quotas", "codex_quotas", "cooldowns", "failure_counts",
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
	{
		Version:     2,
		Description: "Add index on proxy_requests.provider_id",
		Up: func(db *gorm.DB) error {
			// 说明：这是高频列表/过滤路径的关键优化点。
			// 不同数据库方言对 IF NOT EXISTS 的支持不同，这里做最小兼容处理。
			switch db.Dialector.Name() {
			case "mysql":
				err := db.Exec("CREATE INDEX idx_proxy_requests_provider_id ON proxy_requests(provider_id)").Error
				if isMySQLDuplicateIndexError(err) {
					return nil
				}
				return err
			default:
				return db.Exec("CREATE INDEX IF NOT EXISTS idx_proxy_requests_provider_id ON proxy_requests(provider_id)").Error
			}
		},
		Down: func(db *gorm.DB) error {
			switch db.Dialector.Name() {
			case "mysql":
				// MySQL 不支持 DROP INDEX IF EXISTS；这里尽量执行，失败则忽略（回滚不是主路径）。
				sql := "DROP INDEX idx_proxy_requests_provider_id ON proxy_requests"
				if err := db.Exec(sql).Error; err != nil {
					log.Printf("[Migration] Warning: rollback v2 failed (dialector=mysql) sql=%q err=%v", sql, err)
				}
				return nil
			default:
				return db.Exec("DROP INDEX IF EXISTS idx_proxy_requests_provider_id").Error
			}
		},
	},
	{
		Version:     3,
		Description: "Multi-tenancy support: create default tenant and populate tenant_id for all existing data",
		Up: func(db *gorm.DB) error {
			// 1. Insert default tenant if not exists
			var tenantCount int64
			if err := db.Raw("SELECT COUNT(*) FROM tenants WHERE is_default = 1").Scan(&tenantCount).Error; err != nil {
				// Table might not exist yet (AutoMigrate runs before migrations)
				// If tenants table doesn't exist, GORM AutoMigrate should have created it
				return err
			}
			if tenantCount == 0 {
				now := time.Now().UnixMilli()
				if err := db.Exec(
					"INSERT INTO tenants (id, created_at, updated_at, deleted_at, name, slug, is_default) VALUES (?, ?, ?, 0, ?, ?, 1)",
					1, now, now, "Default", "default",
				).Error; err != nil {
					return err
				}
				log.Printf("[Migration] Created default tenant (id=1)")
			}

			// 2. Update all existing rows to belong to default tenant
			for _, table := range tenantScopedTables {
				result := db.Exec("UPDATE " + table + " SET tenant_id = 1 WHERE tenant_id = 0 OR tenant_id IS NULL")
				if result.Error != nil {
					log.Printf("[Migration] Warning: Failed to update tenant_id for %s: %v", table, result.Error)
					// Continue with other tables
				} else if result.RowsAffected > 0 {
					log.Printf("[Migration] Updated %d rows in %s with default tenant_id", result.RowsAffected, table)
				}
			}

			// 3. Generate JWT secret and store in system_settings if not exists
			var jwtSecretCount int64
			db.Raw("SELECT COUNT(*) FROM system_settings WHERE setting_key = 'jwt_secret'").Scan(&jwtSecretCount)
			if jwtSecretCount == 0 {
				// Generate 32-byte random hex secret
				secret := make([]byte, 32)
				if _, err := rand.Read(secret); err != nil {
					return err
				}
				now := time.Now().UnixMilli()
				if err := db.Exec(
					"INSERT INTO system_settings (setting_key, value, created_at, updated_at) VALUES (?, ?, ?, ?)",
					"jwt_secret", hex.EncodeToString(secret), now, now,
				).Error; err != nil {
					return err
				}
				log.Printf("[Migration] Generated JWT secret")
			}

			return nil
		},
		Down: func(db *gorm.DB) error {
			// Rollback: remove jwt_secret setting, but keep tenant_id data
			// (removing tenant_id columns would require dropping and recreating tables)
			db.Exec("DELETE FROM system_settings WHERE setting_key = 'jwt_secret'")
			return nil
		},
	},
	{
		Version:     4,
		Description: "Set status='active' for all existing users",
		Up: func(db *gorm.DB) error {
			return db.Exec("UPDATE users SET status = 'active' WHERE status = '' OR status IS NULL OR status = 'pending'").Error
		},
		Down: func(db *gorm.DB) error {
			return nil
		},
	},
	{
		Version:     5,
		Description: "Hard-delete soft-deleted users to free username unique constraint",
		Up: func(db *gorm.DB) error {
			result := db.Exec("DELETE FROM users WHERE deleted_at != 0")
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected > 0 {
				log.Printf("[Migration] Purged %d soft-deleted users", result.RowsAffected)
			}
			return nil
		},
		Down: func(db *gorm.DB) error {
			return errors.New("migration v5 is irreversible: hard-deleted users cannot be restored")
		},
	},
	{
		Version:     6,
		Description: "Repair tenant_id zeroed by PUT handler bug (providers, projects, retry_configs, routing_strategies)",
		Up: func(db *gorm.DB) error {
			var defaultTenantID uint64
			err := db.Raw("SELECT id FROM tenants WHERE is_default = 1 LIMIT 1").Scan(&defaultTenantID).Error
			if err != nil || defaultTenantID == 0 {
				defaultTenantID = 1
			}

			tables := []string{"providers", "projects", "retry_configs", "routing_strategies"}
			for _, table := range tables {
				result := db.Exec("UPDATE "+table+" SET tenant_id = ? WHERE tenant_id = 0", defaultTenantID)
				if result.Error != nil {
					log.Printf("[Migration] Warning: failed to repair tenant_id for %s: %v", table, result.Error)
				} else if result.RowsAffected > 0 {
					log.Printf("[Migration] Repaired %d rows in %s with tenant_id=%d", result.RowsAffected, table, defaultTenantID)
				}
			}
			return nil
		},
		Down: func(db *gorm.DB) error {
			return nil
		},
	}, {
		Version:     7,
		Description: "Backfill providers.exclude_from_export defaults to 0",
		Up: func(db *gorm.DB) error {
			if !db.Migrator().HasColumn(&Provider{}, "exclude_from_export") {
				return nil
			}
			return db.Exec("UPDATE providers SET exclude_from_export = 0 WHERE exclude_from_export IS NULL").Error
		},
		Down: func(db *gorm.DB) error {
			return nil
		},
	},
	{
		Version:     8,
		Description: "Make Codex quota identity account-aware to avoid same-email quota collisions",
		Up: func(db *gorm.DB) error {
			return applyCodexQuotaIdentityMigration(db)
		},
		Down: func(db *gorm.DB) error {
			return revertCodexQuotaIdentityMigration(db)
		},
	},
	{
		Version:     9,
		Description: "Dedupe codex quota identities and harden index migration ordering",
		Up: func(db *gorm.DB) error {
			return applyCodexQuotaIdentityMigration(db)
		},
		Down: func(db *gorm.DB) error {
			return revertCodexQuotaIdentityMigration(db)
		},
	},
	{
		Version:     10,
		Description: "Add sessions cleanup index on deleted_at and updated_at",
		Up: func(db *gorm.DB) error {
			switch db.Dialector.Name() {
			case "mysql":
				err := db.Exec("CREATE INDEX idx_sessions_deleted_updated_at ON sessions(deleted_at, updated_at)").Error
				if isMySQLDuplicateIndexError(err) {
					return nil
				}
				return err
			default:
				return db.Exec("CREATE INDEX IF NOT EXISTS idx_sessions_deleted_updated_at ON sessions(deleted_at, updated_at)").Error
			}
		},
		Down: func(db *gorm.DB) error {
			switch db.Dialector.Name() {
			case "mysql":
				if err := db.Exec("DROP INDEX idx_sessions_deleted_updated_at ON sessions").Error; err != nil && !isMySQLMissingIndexError(err) {
					return err
				}
				return nil
			default:
				return db.Exec("DROP INDEX IF EXISTS idx_sessions_deleted_updated_at").Error
			}
		},
	},
	{
		Version:     11,
		Description: "Reorder sessions cleanup index to lead with updated_at",
		Up: func(db *gorm.DB) error {
			switch db.Dialector.Name() {
			case "mysql":
				if err := db.Exec("DROP INDEX idx_sessions_deleted_updated_at ON sessions").Error; err != nil && !isMySQLMissingIndexError(err) {
					return err
				}
				err := db.Exec("CREATE INDEX idx_sessions_updated_deleted_at ON sessions(updated_at, deleted_at)").Error
				if isMySQLDuplicateIndexError(err) {
					return nil
				}
				return err
			default:
				if err := db.Exec("DROP INDEX IF EXISTS idx_sessions_deleted_updated_at").Error; err != nil {
					return err
				}
				return db.Exec("CREATE INDEX IF NOT EXISTS idx_sessions_updated_deleted_at ON sessions(updated_at, deleted_at)").Error
			}
		},
		Down: func(db *gorm.DB) error {
			switch db.Dialector.Name() {
			case "mysql":
				if err := db.Exec("DROP INDEX idx_sessions_updated_deleted_at ON sessions").Error; err != nil && !isMySQLMissingIndexError(err) {
					return err
				}
				err := db.Exec("CREATE INDEX idx_sessions_deleted_updated_at ON sessions(deleted_at, updated_at)").Error
				if isMySQLDuplicateIndexError(err) {
					return nil
				}
				return err
			default:
				if err := db.Exec("DROP INDEX IF EXISTS idx_sessions_updated_deleted_at").Error; err != nil {
					return err
				}
				return db.Exec("CREATE INDEX IF NOT EXISTS idx_sessions_deleted_updated_at ON sessions(deleted_at, updated_at)").Error
			}
		},
	},
	{
		Version:     12,
		Description: "Add model column to cooldowns and failure_counts for model-level cooldown granularity",
		Up: func(db *gorm.DB) error {
			// Drop old unique indexes and recreate with model column.
			// Keep indexed string columns short enough for MySQL utf8mb4 composite-key limits.
			switch db.Dialector.Name() {
			case "mysql":
				if err := db.Exec("ALTER TABLE cooldowns ADD COLUMN model VARCHAR(191) DEFAULT ''").Error; err != nil {
					if !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
						return err
					}
				}
				if err := db.Exec("ALTER TABLE failure_counts ADD COLUMN model VARCHAR(191) DEFAULT ''").Error; err != nil {
					if !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
						return err
					}
				}
				if err := db.Exec("ALTER TABLE cooldowns MODIFY COLUMN client_type VARCHAR(64)").Error; err != nil {
					return err
				}
				if err := db.Exec("ALTER TABLE cooldowns MODIFY COLUMN model VARCHAR(191) DEFAULT ''").Error; err != nil {
					return err
				}
				if err := db.Exec("ALTER TABLE failure_counts MODIFY COLUMN client_type VARCHAR(64)").Error; err != nil {
					return err
				}
				if err := db.Exec("ALTER TABLE failure_counts MODIFY COLUMN reason VARCHAR(64)").Error; err != nil {
					return err
				}
				if err := db.Exec("ALTER TABLE failure_counts MODIFY COLUMN model VARCHAR(191) DEFAULT ''").Error; err != nil {
					return err
				}
				if err := db.Exec("CREATE UNIQUE INDEX idx_cooldowns_provider_client_model ON cooldowns(provider_id, client_type, model)").Error; err != nil && !isMySQLDuplicateIndexError(err) {
					return err
				}
				if err := db.Exec("CREATE UNIQUE INDEX idx_failure_counts_tenant_provider_client_reason_model ON failure_counts(tenant_id, provider_id, client_type, reason, model)").Error; err != nil && !isMySQLDuplicateIndexError(err) {
					return err
				}
				if err := db.Exec("DROP INDEX idx_cooldowns_provider_client ON cooldowns").Error; err != nil && !isMySQLMissingIndexError(err) {
					return err
				}
				if err := db.Exec("DROP INDEX idx_failure_counts_tenant_provider_client_reason ON failure_counts").Error; err != nil && !isMySQLMissingIndexError(err) {
					return err
				}
			default:
				if err := db.Exec("DROP INDEX IF EXISTS idx_cooldowns_provider_client").Error; err != nil {
					return err
				}
				if err := db.Exec("DROP INDEX IF EXISTS idx_failure_counts_tenant_provider_client_reason").Error; err != nil {
					return err
				}
				if err := db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_cooldowns_provider_client_model ON cooldowns(provider_id, client_type, model)").Error; err != nil {
					return err
				}
				if err := db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_failure_counts_tenant_provider_client_reason_model ON failure_counts(tenant_id, provider_id, client_type, reason, model)").Error; err != nil {
					return err
				}
			}
			return nil
		},
		Down: func(db *gorm.DB) error {
			return fmt.Errorf("migration v12 cannot be rolled back: dropping columns not supported in SQLite")
		},
	},
	{
		Version:     13,
		Description: "Add detail-cleanup indexes on proxy_requests and proxy_upstream_attempts",
		Up: func(db *gorm.DB) error {
			// detail 清理 (ClearDetailOlderThan) 用
			//   WHERE created_at < ? AND (request_info IS NOT NULL OR response_info IS NOT NULL) [AND dev_mode = 0]
			//   ORDER BY id LIMIT ?
			// 之前没索引时每次都全表扫，长时间占 SQLite 单写锁导致线上写入"卡死"。
			//
			// SQLite 用 **partial index**：WHERE 子句让只有"还没清理"的行进入索引；行被 null
			// 后自动从索引移除，存量收敛后索引很小，scan 几乎免费。键设计 (created_at, id) 同
			// 时服务 cleanup 的 ORDER BY id LIMIT。
			//
			// MySQL 不支持 partial index（functional index 语法又不同），退化为普通的
			// (created_at, id) 复合索引。
			//
			// 大表写锁规避：SQLite 没有 online CREATE INDEX。为不在升级路径上长时间阻塞
			// 写入，runDetailCleanupIndexMigration 对超过 detailCleanupIndexRowThreshold
			// 的表跳过自动建立，打印 manual SQL 由运维选维护窗口跑。
			return runDetailCleanupIndexMigration(db)
		},
		Down: func(db *gorm.DB) error {
			switch db.Dialector.Name() {
			case "mysql":
				for _, sql := range []string{
					"DROP INDEX idx_proxy_requests_detail_cleanup ON proxy_requests",
					"DROP INDEX idx_proxy_upstream_attempts_detail_cleanup ON proxy_upstream_attempts",
				} {
					if err := db.Exec(sql).Error; err != nil && !isMySQLMissingIndexError(err) {
						log.Printf("[Migration] Warning: rollback v13 failed sql=%q err=%v", sql, err)
					}
				}
				return nil
			default:
				if err := db.Exec("DROP INDEX IF EXISTS idx_proxy_requests_detail_cleanup").Error; err != nil {
					return err
				}
				return db.Exec("DROP INDEX IF EXISTS idx_proxy_upstream_attempts_detail_cleanup").Error
			}
		},
	},
	{
		Version:     14,
		Description: "Re-order MySQL proxy_requests detail-cleanup index to (status, dev_mode, created_at, id)",
		Up: func(db *gorm.DB) error {
			// 生产反馈：v13 在 MySQL 上建的 (created_at, id) 复合索引，对清理 SELECT
			// 的 EXPLAIN filtered 只有 ~10%(WHERE 还有 status IN AND dev_mode = 0,
			// 索引前缀覆盖不到要回表)。改成 (status, dev_mode, created_at, id) 后
			// filtered 升到 ~99%，SELECT 从 30-70s 降到亚秒级。
			//
			// SQLite 用 partial index 已把谓词烧进索引,不需要改;attempts 表清理
			// 没有 status/dev_mode 谓词,(created_at, id) 已最优,也保持不变。
			//
			// 大表上 CREATE INDEX 同样可能长时间阻塞写入,沿用 v13 的 threshold-skip
			// 策略:超过 detailCleanupIndexRowThreshold 行打印 manual SQL,由运维选窗口跑。
			if db.Dialector.Name() != "mysql" {
				return nil
			}
			return runDetailCleanupIndexV2Migration(db)
		},
		Down: func(db *gorm.DB) error {
			if db.Dialector.Name() != "mysql" {
				return nil
			}
			// 回滚:删 v2 名字的索引并尝试恢复 v13 的 (created_at, id)
			if err := db.Exec("DROP INDEX idx_proxy_requests_detail_cleanup_v2 ON proxy_requests").Error; err != nil && !isMySQLMissingIndexError(err) {
				log.Printf("[Migration] Warning: rollback v14 drop v2 failed: %v", err)
			}
			err := db.Exec("CREATE INDEX idx_proxy_requests_detail_cleanup ON proxy_requests(created_at, id)").Error
			if err != nil && !isMySQLDuplicateIndexError(err) {
				log.Printf("[Migration] Warning: rollback v14 recreate v1 failed: %v", err)
			}
			return nil
		},
	},
	{
		Version:     15,
		Description: "Add detail_cleared sentinel column + index for cleanup (threshold-skipped on large tables)",
		Up: func(db *gorm.DB) error {
			// v15 引入 detail_cleared TINYINT sentinel + 复合索引 (detail_cleared, created_at, id)。
			// 配合 ClearDetailOlderThan 的 WHERE detail_cleared = 0,planner 走 leading-column
			// 等值匹配,不必回行评估 LONGTEXT NULL flag。生产上原 43-min stall 的根因。
			//
			// **detail_cleared 列由这里的 raw SQL 添加**,不放到 GORM struct 上:
			// - AutoMigrate 在 RunMigrations 之前运行,放到 struct 上就绕开了 threshold 守护
			// - 46GB 大表上 5.7 INPLACE ALTER 可能小时级阻塞
			// - 这里加 threshold-skip + bare ALTER TABLE,运维超阈值时手动跑
			if err := runDetailClearedColumnMigration(db); err != nil {
				return err
			}
			return runDetailClearedIndexMigration(db)
		},
		Down: func(db *gorm.DB) error {
			// 索引 drop 优先,column 保留(列 DROP 在 MySQL 5.7 / 大表上同样危险,运维手动)。
			switch db.Dialector.Name() {
			case "mysql":
				for _, sql := range []string{
					"DROP INDEX idx_proxy_requests_detail_cleared ON proxy_requests",
					"DROP INDEX idx_proxy_upstream_attempts_detail_cleared ON proxy_upstream_attempts",
				} {
					if err := db.Exec(sql).Error; err != nil && !isMySQLMissingIndexError(err) {
						log.Printf("[Migration] Warning: rollback v15 failed sql=%q err=%v", sql, err)
					}
				}
				return nil
			default:
				if err := db.Exec("DROP INDEX IF EXISTS idx_proxy_requests_detail_cleared").Error; err != nil {
					return err
				}
				return db.Exec("DROP INDEX IF EXISTS idx_proxy_upstream_attempts_detail_cleared").Error
			}
		},
	},
}

// runDetailClearedColumnMigration 显式添加 detail_cleared 列到两张大表,带 threshold-skip。
//
// 设计要点:
//   - bare ALTER TABLE ADD COLUMN,无 ALGORITHM/LOCK hint(吸取 v14 1064 教训)。MySQL 5.7+
//     8.0+ 自动选最优算法
//   - **列类型显式 TINYINT NOT NULL DEFAULT 0**:GORM 默认会把 Go int 转成 bigint,46GB 表上
//     每行多耗 7 字节没必要
//   - 沿用 v13/v14 的 500k 行 threshold:超过则打印 manual SQL,migration 仍标记完成
//   - 幂等:列已存在则跳过(检测错误码 / 文本)
func runDetailClearedColumnMigration(db *gorm.DB) error {
	type tableSpec struct {
		table string
		// 列检测的 SQL 与 dialect 相关,统一封装
	}
	tables := []string{"proxy_requests", "proxy_upstream_attempts"}

	columnExists := func(table string) (bool, error) {
		switch db.Dialector.Name() {
		case "mysql":
			var n int64
			err := db.Raw(`
				SELECT COUNT(*) FROM information_schema.COLUMNS
				WHERE table_schema = DATABASE() AND table_name = ? AND column_name = 'detail_cleared'
			`, table).Scan(&n).Error
			return n > 0, err
		case "postgres":
			var n int64
			err := db.Raw(`
				SELECT COUNT(*) FROM information_schema.columns
				WHERE table_schema = current_schema() AND table_name = ? AND column_name = 'detail_cleared'
			`, table).Scan(&n).Error
			return n > 0, err
		default:
			// SQLite:PRAGMA table_info(...) 列表里找。
			// 注意:**不要**让 PRAGMA 跑在其它 dialect 上——Postgres 会以 SQLSTATE 42601
			// 抛出错误并 abort 整个事务,导致 v15 后续 SQL 全部失败。multiinstance CI 抓到。
			rows, err := db.Raw("PRAGMA table_info(" + table + ")").Rows()
			if err != nil {
				return false, err
			}
			defer rows.Close()
			for rows.Next() {
				var cid int
				var name, ctype string
				var notnull, pk int
				var dflt any
				if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
					return false, err
				}
				if name == "detail_cleared" {
					return true, nil
				}
			}
			return false, nil
		}
	}

	for _, table := range tables {
		exists, err := columnExists(table)
		if err != nil {
			log.Printf("[Migration v15] could not probe %s.detail_cleared (%v); skipping column add. Apply manually if needed.", table, err)
			continue
		}
		if exists {
			continue
		}

		var rowCount int64
		if err := db.Raw("SELECT COUNT(*) FROM " + table).Scan(&rowCount).Error; err != nil {
			log.Printf("[Migration v15] could not count %s (%v); skipping column add.", table, err)
			continue
		}

		// 列类型按 dialect:
		//   - MySQL:TINYINT 1 字节
		//   - Postgres:SMALLINT 2 字节(没有 TINYINT,SMALLINT 是 16-bit 最小整型)
		//   - SQLite:整型亲和力,类型名只是 hint,用 TINYINT 与 MySQL 对齐即可
		columnType := "TINYINT"
		if db.Dialector.Name() == "postgres" {
			columnType = "SMALLINT"
		}
		ddl := "ALTER TABLE " + table + " ADD COLUMN detail_cleared " + columnType + " NOT NULL DEFAULT 0"

		if rowCount > detailCleanupIndexRowThreshold {
			log.Printf("[Migration v15] SKIPPING ADD COLUMN on %s: %d rows (> %d threshold). "+
				"Apply manually during a maintenance window:\n  %s;",
				table, rowCount, detailCleanupIndexRowThreshold, ddl)
			continue
		}

		log.Printf("[Migration v15] adding %s.detail_cleared column (rows=%d)", table, rowCount)
		start := time.Now()
		if err := db.Exec(ddl).Error; err != nil {
			// 列已存在的 MySQL/SQLite 错误吞掉(幂等),其它错误抛出
			lower := strings.ToLower(err.Error())
			if strings.Contains(lower, "duplicate column") || strings.Contains(lower, "already exists") {
				log.Printf("[Migration v15] %s.detail_cleared already exists, skipped", table)
				continue
			}
			return err
		}
		log.Printf("[Migration v15] added %s.detail_cleared in %s", table, time.Since(start).Round(time.Millisecond))
	}
	return nil
}

// runDetailClearedIndexMigration 建立 v15 的 sentinel 复合索引。
//
// 设计要点:
//   - 不带 ALGORITHM/LOCK hint:运维强烈要求避免 dialect-specific 语法。MySQL 自己挑算法,
//     5.7 / 8.0 都跑得通(普通 CREATE INDEX 在两版都是 INPLACE)
//   - 沿用 v13/v14 的 threshold-skip(500k 行):大表升级路径仍打 manual SQL,运维窗口手动跑
//   - 不写 SQLite 的 partial index:用普通复合索引,代码路径统一,两 dialect 一致
//   - 已有同名索引就跳过(幂等)
func runDetailClearedIndexMigration(db *gorm.DB) error {
	type tableSpec struct {
		table string
		index string
		ddl   string
	}
	specs := []tableSpec{
		{
			table: "proxy_requests",
			index: "idx_proxy_requests_detail_cleared",
			ddl:   "CREATE INDEX idx_proxy_requests_detail_cleared ON proxy_requests(detail_cleared, created_at, id)",
		},
		{
			table: "proxy_upstream_attempts",
			index: "idx_proxy_upstream_attempts_detail_cleared",
			ddl:   "CREATE INDEX idx_proxy_upstream_attempts_detail_cleared ON proxy_upstream_attempts(detail_cleared, created_at, id)",
		},
	}

	for _, s := range specs {
		var rowCount int64
		if err := db.Raw("SELECT COUNT(*) FROM " + s.table).Scan(&rowCount).Error; err != nil {
			log.Printf("[Migration v15] could not count %s (%v); skipping %s. Apply manually if needed.", s.table, err, s.index)
			continue
		}

		ddl := s.ddl
		if db.Dialector.Name() != "mysql" {
			// SQLite 支持 IF NOT EXISTS,显式加上让重复 migration 不报错。
			ddl = strings.Replace(ddl, "CREATE INDEX", "CREATE INDEX IF NOT EXISTS", 1)
		}

		if rowCount > detailCleanupIndexRowThreshold {
			log.Printf("[Migration v15] SKIPPING %s build: %s has %d rows (> %d threshold). "+
				"Apply manually during a maintenance window:\n  %s;",
				s.index, s.table, rowCount, detailCleanupIndexRowThreshold, s.ddl)
			continue
		}

		log.Printf("[Migration v15] building %s (rows=%d)", s.index, rowCount)
		start := time.Now()
		if err := db.Exec(ddl).Error; err != nil {
			if db.Dialector.Name() == "mysql" && isMySQLDuplicateIndexError(err) {
				log.Printf("[Migration v15] %s already exists, skipped", s.index)
				continue
			}
			return err
		}
		log.Printf("[Migration v15] built %s in %s", s.index, time.Since(start).Round(time.Millisecond))
	}
	return nil
}

// runDetailCleanupIndexV2Migration 创建 (status, dev_mode, created_at, id) 索引并删除旧的
// (created_at, id) 索引(同一张表两个清理索引同时存在意义不大,且 MySQL writes 要维护两份)。
//
// 命名为 _v2 后缀,保留旧名 idx_proxy_requests_detail_cleanup 给运维以便回滚/对照;
// drop 顺序在 create 之后,确保任何瞬间至少一个索引可用(planner 会自动切换)。
//
// 大表跳过:超过阈值则只打印 manual SQL,migration 仍标记完成。降级行为:planner 退化
// 用旧 (created_at, id) 索引,SELECT 慢但不至于卡死。
func runDetailCleanupIndexV2Migration(db *gorm.DB) error {
	const (
		oldIndex = "idx_proxy_requests_detail_cleanup"
		newIndex = "idx_proxy_requests_detail_cleanup_v2"
		// ALGORITHM=INPLACE, LOCK=NONE 显式声明 online DDL。这两个子句**只能用在
		// ALTER TABLE ADD INDEX 上**;附在 CREATE INDEX 后面 MySQL 8 会报 1064 语法错。
		// 之前用 CREATE INDEX 的版本在 v0.13.78 启动崩溃(生产报告)。
		// 用 ALTER TABLE 等价表达,等价语义,但语法被 MySQL parser 接受。
		createSQL = "ALTER TABLE proxy_requests ADD INDEX idx_proxy_requests_detail_cleanup_v2 (status, dev_mode, created_at, id), ALGORITHM=INPLACE, LOCK=NONE"
		// manual SQL log 中保持 CREATE INDEX 形式:运维窗口下不需要 INPLACE/LOCK 提示。
		manualCreateSQL = "CREATE INDEX idx_proxy_requests_detail_cleanup_v2 ON proxy_requests(status, dev_mode, created_at, id)"
	)
	var rowCount int64
	if err := db.Raw("SELECT COUNT(*) FROM proxy_requests").Scan(&rowCount).Error; err != nil {
		log.Printf("[Migration v14] could not count proxy_requests (%v); skipping index build. Apply manually if needed.", err)
		return nil
	}

	if rowCount > detailCleanupIndexRowThreshold {
		log.Printf("[Migration v14] SKIPPING %s build: proxy_requests has %d rows (> %d threshold). "+
			"CREATE INDEX would block writes. Apply manually during a maintenance window:\n"+
			"  %s;\n"+
			"  -- then (only if v13 had successfully built the old index):\n"+
			"  -- DROP INDEX %s ON proxy_requests;",
			newIndex, rowCount, detailCleanupIndexRowThreshold, manualCreateSQL, oldIndex)
		return nil
	}

	log.Printf("[Migration v14] building %s (rows=%d)", newIndex, rowCount)
	start := time.Now()
	if err := db.Exec(createSQL).Error; err != nil && !isMySQLDuplicateIndexError(err) {
		return err
	}
	log.Printf("[Migration v14] built %s in %s", newIndex, time.Since(start).Round(time.Millisecond))

	if err := db.Exec("DROP INDEX " + oldIndex + " ON proxy_requests").Error; err != nil && !isMySQLMissingIndexError(err) {
		log.Printf("[Migration v14] Warning: drop old %s failed (non-fatal): %v", oldIndex, err)
	}
	return nil
}

// detailCleanupIndexRowThreshold 是 v13 自动建索引的行数上限。超过它则跳过自动建立
// 并打印 manual SQL，由运维选维护窗口手动执行——避免大表升级时数据库被 CREATE INDEX
// 长时间独占写锁（SQLite 没有 online DDL）。500k 行是经验值：低于此 SSD 上建索引秒级完成，
// 远高于这个量级时建索引耗时可达数十秒到分钟级。
const detailCleanupIndexRowThreshold = 500_000

// runDetailCleanupIndexMigration 建立 v13 的清理索引：
//   - 小表：直接同步建立（耗时可忽略）。
//   - 大表：跳过，但打印明确的 manual SQL + 行数，让运维选窗口手动跑。
//
// 跳过分支下 migration 仍记为已应用——v14+ 不应依赖该索引存在。清理路径在缺索引时
// 退化到全表扫，但 batch+sleep 的 yield 已经能避免"卡死"，只是稳态扫描成本高一些。
func runDetailCleanupIndexMigration(db *gorm.DB) error {
	type tableSpec struct {
		table     string
		index     string
		sqliteDDL string
		mysqlDDL  string
	}
	specs := []tableSpec{
		{
			table:     "proxy_requests",
			index:     "idx_proxy_requests_detail_cleanup",
			sqliteDDL: "CREATE INDEX IF NOT EXISTS idx_proxy_requests_detail_cleanup ON proxy_requests(created_at, id) WHERE (request_info IS NOT NULL OR response_info IS NOT NULL) AND dev_mode = 0",
			mysqlDDL:  "CREATE INDEX idx_proxy_requests_detail_cleanup ON proxy_requests(created_at, id)",
		},
		{
			table:     "proxy_upstream_attempts",
			index:     "idx_proxy_upstream_attempts_detail_cleanup",
			sqliteDDL: "CREATE INDEX IF NOT EXISTS idx_proxy_upstream_attempts_detail_cleanup ON proxy_upstream_attempts(created_at, id) WHERE request_info IS NOT NULL OR response_info IS NOT NULL",
			mysqlDDL:  "CREATE INDEX idx_proxy_upstream_attempts_detail_cleanup ON proxy_upstream_attempts(created_at, id)",
		},
	}

	for _, s := range specs {
		var rowCount int64
		// 表名是包内硬编码字面量，不存在注入面；GORM 占位符不支持表名。
		if err := db.Raw("SELECT COUNT(*) FROM " + s.table).Scan(&rowCount).Error; err != nil {
			// 计数失败时保守起见跳过——宁可慢，不要在升级路径上炸。
			log.Printf("[Migration v13] could not count %s (%v); skipping index build. Apply manually if needed.", s.table, err)
			continue
		}

		var ddl string
		switch db.Dialector.Name() {
		case "mysql":
			ddl = s.mysqlDDL
		default:
			ddl = s.sqliteDDL
		}

		if rowCount > detailCleanupIndexRowThreshold {
			log.Printf("[Migration v13] SKIPPING %s build: %s has %d rows (> %d threshold). "+
				"CREATE INDEX would block writes for tens of seconds. "+
				"Apply manually during a maintenance window:\n  %s",
				s.index, s.table, rowCount, detailCleanupIndexRowThreshold, ddl)
			continue
		}

		log.Printf("[Migration v13] building %s (rows=%d)", s.index, rowCount)
		start := time.Now()
		if err := db.Exec(ddl).Error; err != nil {
			if db.Dialector.Name() == "mysql" && isMySQLDuplicateIndexError(err) {
				log.Printf("[Migration v13] %s already exists, skipped", s.index)
				continue
			}
			return err
		}
		log.Printf("[Migration v13] built %s in %s", s.index, time.Since(start).Round(time.Millisecond))
	}
	return nil
}

func applyCodexQuotaIdentityMigration(db *gorm.DB) error {
	if !db.Migrator().HasColumn(&CodexQuota{}, "identity_key") {
		return nil
	}
	if err := dropCodexQuotaIdentityIndexesBeforeBackfill(db); err != nil {
		return err
	}
	if err := db.Exec(codexQuotaIdentityBackfillSQL(db.Dialector.Name())).Error; err != nil {
		return err
	}
	if err := dedupeCodexQuotaIdentityRows(db); err != nil {
		return err
	}
	return ensureCodexQuotaIdentityIndexes(db)
}

func revertCodexQuotaIdentityMigration(db *gorm.DB) error {
	if !db.Migrator().HasColumn(&CodexQuota{}, "identity_key") {
		return nil
	}
	return fmt.Errorf("reverting codex quota identity migration is irreversible: cannot safely recreate idx_codex_quotas_tenant_email because CodexQuota rows may now contain duplicate (tenant_id, email) values after identity/email migration")
}

func dedupeCodexQuotaIdentityRows(db *gorm.DB) error {
	return db.Exec(codexQuotaIdentityDedupeSQL(db.Dialector.Name())).Error
}

func codexQuotaIdentityBackfillSQL(dialector string) string {
	switch dialector {
	case "mysql":
		return `
			UPDATE codex_quotas
			SET identity_key = CASE
				WHEN account_id IS NOT NULL AND TRIM(account_id) != '' THEN CONCAT('account:', TRIM(account_id))
				WHEN email IS NOT NULL AND TRIM(email) != '' THEN CONCAT('email:', TRIM(email))
				ELSE NULL
			END
			WHERE identity_key IS NULL OR TRIM(identity_key) = ''
		`
	default:
		return `
			UPDATE codex_quotas
			SET identity_key = CASE
				WHEN account_id IS NOT NULL AND TRIM(account_id) != '' THEN 'account:' || TRIM(account_id)
				WHEN email IS NOT NULL AND TRIM(email) != '' THEN 'email:' || TRIM(email)
				ELSE NULL
			END
			WHERE identity_key IS NULL OR TRIM(identity_key) = ''
		`
	}
}

func codexQuotaIdentityDedupeSQL(dialector string) string {
	switch dialector {
	case "mysql":
		return `
			DELETE doomed
			FROM codex_quotas AS doomed
			JOIN codex_quotas AS keeper
			  ON doomed.tenant_id = keeper.tenant_id
			 AND doomed.identity_key = keeper.identity_key
			 AND COALESCE(doomed.deleted_at, 0) = COALESCE(keeper.deleted_at, 0)
			 AND (
				COALESCE(keeper.updated_at, 0) > COALESCE(doomed.updated_at, 0)
				OR (
					COALESCE(keeper.updated_at, 0) = COALESCE(doomed.updated_at, 0)
					AND keeper.id < doomed.id
				)
			 )
			WHERE doomed.identity_key IS NOT NULL
			  AND TRIM(doomed.identity_key) != ''
		`
	default:
		return `
			DELETE FROM codex_quotas
			WHERE id IN (
				SELECT doomed.id
				FROM codex_quotas AS doomed
				JOIN codex_quotas AS keeper
				  ON doomed.tenant_id = keeper.tenant_id
				 AND doomed.identity_key = keeper.identity_key
				 AND COALESCE(doomed.deleted_at, 0) = COALESCE(keeper.deleted_at, 0)
				 AND (
					COALESCE(keeper.updated_at, 0) > COALESCE(doomed.updated_at, 0)
					OR (
						COALESCE(keeper.updated_at, 0) = COALESCE(doomed.updated_at, 0)
						AND keeper.id < doomed.id
					)
				 )
				WHERE doomed.identity_key IS NOT NULL
				  AND TRIM(doomed.identity_key) != ''
			)
		`
	}
}

func ensureCodexQuotaIdentityIndexes(db *gorm.DB) error {
	switch db.Dialector.Name() {
	case "mysql":
		if err := db.Exec("CREATE UNIQUE INDEX idx_codex_quotas_tenant_identity ON codex_quotas(tenant_id, identity_key, deleted_at)").Error; err != nil && !isMySQLDuplicateIndexError(err) {
			return err
		}
		if err := db.Exec("DROP INDEX idx_codex_quotas_tenant_email ON codex_quotas").Error; err != nil && !isMySQLMissingIndexError(err) {
			return err
		}
		if err := db.Exec("DROP INDEX idx_codex_quotas_email ON codex_quotas").Error; err != nil && !isMySQLMissingIndexError(err) {
			return err
		}
		if err := db.Exec("CREATE INDEX idx_codex_quotas_email ON codex_quotas(email)").Error; err != nil && !isMySQLDuplicateIndexError(err) {
			return err
		}
	case "postgres":
		if err := db.Exec(`
			CREATE UNIQUE INDEX IF NOT EXISTS idx_codex_quotas_tenant_identity
			ON codex_quotas(tenant_id, identity_key)
			WHERE deleted_at = 0 AND identity_key IS NOT NULL AND TRIM(identity_key) != ''
		`).Error; err != nil {
			return err
		}
		if err := db.Exec("DROP INDEX IF EXISTS idx_codex_quotas_tenant_email").Error; err != nil {
			return err
		}
		if err := db.Exec("DROP INDEX IF EXISTS idx_codex_quotas_email").Error; err != nil {
			return err
		}
		if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_codex_quotas_email ON codex_quotas(email)").Error; err != nil {
			return err
		}
	default:
		if err := db.Exec(`
			CREATE UNIQUE INDEX IF NOT EXISTS idx_codex_quotas_tenant_identity
			ON codex_quotas(tenant_id, identity_key)
			WHERE deleted_at = 0 AND identity_key IS NOT NULL AND TRIM(identity_key) != ''
		`).Error; err != nil {
			return err
		}
		if err := db.Exec("DROP INDEX IF EXISTS idx_codex_quotas_tenant_email").Error; err != nil {
			return err
		}
		if err := db.Exec("DROP INDEX IF EXISTS idx_codex_quotas_email").Error; err != nil {
			return err
		}
		if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_codex_quotas_email ON codex_quotas(email)").Error; err != nil {
			return err
		}
	}
	return nil
}

func dropCodexQuotaIdentityIndexesBeforeBackfill(db *gorm.DB) error {
	switch db.Dialector.Name() {
	case "mysql":
		if err := db.Exec("DROP INDEX idx_codex_quotas_tenant_identity ON codex_quotas").Error; err != nil && !isMySQLMissingIndexError(err) {
			return err
		}
	case "postgres":
		if err := db.Exec("DROP INDEX IF EXISTS idx_codex_quotas_tenant_identity").Error; err != nil {
			return err
		}
	default:
		if err := db.Exec("DROP INDEX IF EXISTS idx_codex_quotas_tenant_identity").Error; err != nil {
			return err
		}
	}
	return nil
}

func isMySQLDuplicateIndexError(err error) bool {
	if err == nil {
		return false
	}
	var mysqlErr *mysqlDriver.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1061 // ER_DUP_KEYNAME
	}
	// 兜底：错误可能被包装成字符串，避免使用过宽的 "duplicate" 匹配导致吞掉其它错误。
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "duplicate key name") || strings.Contains(lower, "error 1061")
}

func isMySQLMissingIndexError(err error) bool {
	if err == nil {
		return false
	}
	var mysqlErr *mysqlDriver.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1091 // ER_CANT_DROP_FIELD_OR_KEY
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "can't drop") && strings.Contains(lower, "check that column/key exists")
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
	// 注意：MySQL 的 DDL（如 CREATE/DROP INDEX）会触发隐式提交（implicit commit），
	// 这意味着即使这里用 gorm.Transaction 包裹，MySQL 路径也无法提供严格的“DDL + 迁移记录”原子性。
	//
	// 因此迁移实现必须尽量幂等：例如重复执行 CREATE INDEX 时，仅在 ER_DUP_KEYNAME(1061) 场景下视为成功。
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
	// 同 runMigration：MySQL DDL 在回滚路径同样可能发生隐式提交，因此这里的事务主要用于把“回滚逻辑”
	// 与“删除迁移记录”尽量绑定在一起，但不应假设 MySQL 上能做到严格原子回滚。
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
