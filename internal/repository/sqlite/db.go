package sqlite

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type DB struct {
	gorm      *gorm.DB
	dialector string // "sqlite", "mysql", or "postgres"
}

// GormDB returns the underlying GORM DB instance
func (d *DB) GormDB() *gorm.DB {
	return d.gorm
}

// Dialector returns the database dialector type ("sqlite", "mysql", or "postgres")
func (d *DB) Dialector() string {
	return d.dialector
}

// NewDB creates a new database connection
// path: SQLite file path (legacy, for backwards compatibility)
func NewDB(path string) (*DB, error) {
	return NewDBWithDSN("sqlite://" + path)
}

// NewDBWithDSN creates a new database connection using DSN
// DSN formats:
//   - SQLite: "sqlite:///path/to/db.sqlite" or just "/path/to/db.sqlite"
//   - MySQL:  "mysql://user:password@tcp(host:port)/dbname?parseTime=true"
//   - PostgreSQL: "postgres://user:password@host:port/dbname?sslmode=disable"
func NewDBWithDSN(dsn string) (*DB, error) {
	var dialector gorm.Dialector
	var dialectorName string

	if strings.HasPrefix(dsn, "mysql://") {
		// MySQL DSN: mysql://user:password@tcp(host:port)/dbname?parseTime=true
		mysqlDSN := strings.TrimPrefix(dsn, "mysql://")
		dialector = mysql.Open(mysqlDSN)
		dialectorName = "mysql"
		log.Printf("[DB] Connecting to MySQL database")
	} else if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		// PostgreSQL DSN: postgres://user:password@host:port/dbname?sslmode=disable
		dialector = postgres.Open(dsn)
		dialectorName = "postgres"
		log.Printf("[DB] Connecting to PostgreSQL database")
	} else {
		// SQLite DSN: sqlite:///path/to/db.sqlite or just /path/to/db.sqlite
		sqlitePath := strings.TrimPrefix(dsn, "sqlite://")
		// Add SQLite options for WAL mode and busy timeout
		if !strings.Contains(sqlitePath, "?") {
			sqlitePath += "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(30000)"
		}
		dialector = sqlite.Open(sqlitePath)
		dialectorName = "sqlite"
		log.Printf("[DB] Connecting to SQLite database: %s", sqlitePath)
	}

	gormDB, err := gorm.Open(dialector, &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Get underlying sql.DB to verify connection
	sqlDB, err := gormDB.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get sql.DB: %w", err)
	}

	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	d := &DB{gorm: gormDB, dialector: dialectorName}

	// Auto-migrate schema using GORM
	if err := d.autoMigrate(); err != nil {
		return nil, err
	}

	// Run legacy migrations for any schema changes not in GORM models
	if err := d.RunMigrations(); err != nil {
		return nil, err
	}

	if err := d.seedModelMappings(); err != nil {
		return nil, err
	}

	log.Printf("[DB] Database connection established successfully (%s)", dialectorName)
	return d, nil
}

// autoMigrate uses GORM auto-migration
func (d *DB) autoMigrate() error {
	log.Println("[DB] Running GORM auto-migration...")
	return d.gorm.AutoMigrate(AllModels()...)
}

func (d *DB) Close() error {
	sqlDB, err := d.gorm.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// seedModelMappings 种子数据：内置的模型映射规则
func (d *DB) seedModelMappings() error {
	// 检查是否已有规则
	var count int64
	if err := d.gorm.Model(&ModelMapping{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil // 已有规则，跳过
	}

	defaultRules := []ModelMapping{
		{Scope: "global", ClientType: "claude", ProviderType: "antigravity", Pattern: "gpt-4o-mini*", Target: "gemini-2.5-flash", Priority: 0},
		{Scope: "global", ClientType: "claude", ProviderType: "antigravity", Pattern: "gpt-4o*", Target: "gemini-3-flash", Priority: 1},
		{Scope: "global", ClientType: "claude", ProviderType: "antigravity", Pattern: "gpt-4*", Target: "gemini-3-pro-high", Priority: 2},
		{Scope: "global", ClientType: "claude", ProviderType: "antigravity", Pattern: "gpt-3.5*", Target: "gemini-2.5-flash", Priority: 3},
		{Scope: "global", ClientType: "claude", ProviderType: "antigravity", Pattern: "o1-*", Target: "gemini-3-pro-high", Priority: 4},
		{Scope: "global", ClientType: "claude", ProviderType: "antigravity", Pattern: "o3-*", Target: "gemini-3-pro-high", Priority: 5},
		{Scope: "global", ClientType: "claude", ProviderType: "antigravity", Pattern: "claude-3-5-sonnet-*", Target: "claude-sonnet-4-5", Priority: 6},
		{Scope: "global", ClientType: "claude", ProviderType: "antigravity", Pattern: "claude-3-opus-*", Target: "claude-opus-4-5-thinking", Priority: 7},
		{Scope: "global", ClientType: "claude", ProviderType: "antigravity", Pattern: "claude-opus-4-*", Target: "claude-opus-4-5-thinking", Priority: 8},
		{Scope: "global", ClientType: "claude", ProviderType: "antigravity", Pattern: "claude-haiku-*", Target: "gemini-2.5-flash-lite", Priority: 9},
		{Scope: "global", ClientType: "claude", ProviderType: "antigravity", Pattern: "claude-3-haiku-*", Target: "gemini-2.5-flash-lite", Priority: 10},
		{Scope: "global", ClientType: "claude", ProviderType: "antigravity", Pattern: "*opus*", Target: "claude-opus-4-5-thinking", Priority: 11},
		{Scope: "global", ClientType: "claude", ProviderType: "antigravity", Pattern: "*sonnet*", Target: "claude-sonnet-4-5", Priority: 12},
		{Scope: "global", ClientType: "claude", ProviderType: "antigravity", Pattern: "*haiku*", Target: "gemini-2.5-flash-lite", Priority: 13},
	}

	return d.gorm.Create(&defaultRules).Error
}

// ==================== 时间戳辅助函数 ====================

// toTimestamp 将 time.Time 转换为 Unix 毫秒时间戳
func toTimestamp(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

// toTimestampPtr 将 *time.Time 转换为 Unix 毫秒时间戳
func toTimestampPtr(t *time.Time) int64 {
	if t == nil || t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

// fromTimestamp 将 Unix 毫秒时间戳转换为 time.Time
func fromTimestamp(ms int64) time.Time {
	if ms == 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms)
}

// fromTimestampPtr 将 Unix 毫秒时间戳转换为 *time.Time
func fromTimestampPtr(ms int64) *time.Time {
	if ms == 0 {
		return nil
	}
	t := time.UnixMilli(ms)
	return &t
}

// ==================== JSON 辅助函数 ====================

func toJSON(v interface{}) string {
	if v == nil {
		return ""
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func fromJSON[T any](s string) T {
	var v T
	if s != "" {
		json.Unmarshal([]byte(s), &v)
	}
	return v
}
