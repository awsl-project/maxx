package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	db *sql.DB
}

func NewDB(path string) (*DB, error) {
	db, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=30000")
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		return nil, err
	}

	d := &DB{db: db}
	if err := d.migrate(); err != nil {
		return nil, err
	}

	return d, nil
}

func (d *DB) Close() error {
	return d.db.Close()
}

func (d *DB) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return d.db.Query(query, args...)
}

func (d *DB) Exec(query string, args ...interface{}) (sql.Result, error) {
	return d.db.Exec(query, args...)
}

func (d *DB) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS providers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		type TEXT NOT NULL,
		name TEXT NOT NULL,
		config TEXT,
		supported_client_types TEXT,
		deleted_at DATETIME
	);

	CREATE TABLE IF NOT EXISTS projects (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		name TEXT NOT NULL,
		slug TEXT NOT NULL DEFAULT '',
		deleted_at DATETIME
	);

	CREATE TABLE IF NOT EXISTS sessions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		session_id TEXT NOT NULL UNIQUE,
		client_type TEXT NOT NULL,
		project_id INTEGER DEFAULT 0
	);

	CREATE TABLE IF NOT EXISTS routes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		is_enabled INTEGER DEFAULT 1,
		is_native INTEGER DEFAULT 1,
		project_id INTEGER DEFAULT 0,
		client_type TEXT NOT NULL,
		provider_id INTEGER NOT NULL,
		position INTEGER DEFAULT 0,
		retry_config_id INTEGER DEFAULT 0,
		model_mapping TEXT
	);

	CREATE TABLE IF NOT EXISTS retry_configs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		name TEXT NOT NULL,
		is_default INTEGER DEFAULT 0,
		max_retries INTEGER DEFAULT 3,
		initial_interval_ms INTEGER DEFAULT 1000,
		backoff_rate REAL DEFAULT 2.0,
		max_interval_ms INTEGER DEFAULT 30000
	);

	CREATE TABLE IF NOT EXISTS routing_strategies (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		project_id INTEGER DEFAULT 0,
		type TEXT NOT NULL,
		config TEXT
	);

	CREATE TABLE IF NOT EXISTS proxy_requests (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		instance_id TEXT,
		request_id TEXT,
		session_id TEXT,
		client_type TEXT,
		request_model TEXT,
		response_model TEXT,
		start_time DATETIME,
		end_time DATETIME,
		duration_ms INTEGER,
		status TEXT,
		request_info TEXT,
		response_info TEXT,
		error TEXT,
		proxy_upstream_attempt_count INTEGER DEFAULT 0,
		final_proxy_upstream_attempt_id INTEGER DEFAULT 0,
		input_token_count INTEGER DEFAULT 0,
		output_token_count INTEGER DEFAULT 0,
		cache_read_count INTEGER DEFAULT 0,
		cache_write_count INTEGER DEFAULT 0,
		cache_5m_write_count INTEGER DEFAULT 0,
		cache_1h_write_count INTEGER DEFAULT 0,
		cost INTEGER DEFAULT 0,
		route_id INTEGER DEFAULT 0,
		provider_id INTEGER DEFAULT 0,
		is_stream INTEGER DEFAULT 0
	);

	CREATE TABLE IF NOT EXISTS proxy_upstream_attempts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		status TEXT,
		proxy_request_id INTEGER,
		request_info TEXT,
		response_info TEXT,
		route_id INTEGER,
		provider_id INTEGER,
		input_token_count INTEGER DEFAULT 0,
		output_token_count INTEGER DEFAULT 0,
		cache_read_count INTEGER DEFAULT 0,
		cache_write_count INTEGER DEFAULT 0,
		cache_5m_write_count INTEGER DEFAULT 0,
		cache_1h_write_count INTEGER DEFAULT 0,
		cost INTEGER DEFAULT 0,
		is_stream INTEGER DEFAULT 0
	);

	CREATE TABLE IF NOT EXISTS system_settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_sessions_session_id ON sessions(session_id);
	CREATE INDEX IF NOT EXISTS idx_routes_project_client ON routes(project_id, client_type);
	CREATE INDEX IF NOT EXISTS idx_proxy_requests_session ON proxy_requests(session_id);
	CREATE INDEX IF NOT EXISTS idx_proxy_requests_created_at ON proxy_requests(created_at);

	CREATE TABLE IF NOT EXISTS cooldowns (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		provider_id INTEGER NOT NULL,
		client_type TEXT NOT NULL DEFAULT '',
		until_time DATETIME NOT NULL,
		reason TEXT NOT NULL DEFAULT 'unknown'
	);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_cooldowns_provider_client ON cooldowns(provider_id, client_type);
	CREATE INDEX IF NOT EXISTS idx_cooldowns_until ON cooldowns(until_time);

	CREATE TABLE IF NOT EXISTS failure_counts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		provider_id INTEGER NOT NULL,
		client_type TEXT NOT NULL DEFAULT '',
		reason TEXT NOT NULL,
		count INTEGER DEFAULT 0,
		last_failure_at DATETIME NOT NULL
	);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_failure_counts_provider_client_reason ON failure_counts(provider_id, client_type, reason);
	CREATE INDEX IF NOT EXISTS idx_failure_counts_last_failure ON failure_counts(last_failure_at);

	CREATE TABLE IF NOT EXISTS antigravity_quotas (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		email TEXT NOT NULL UNIQUE,
		subscription_tier TEXT DEFAULT 'FREE',
		is_forbidden INTEGER DEFAULT 0,
		models TEXT,
		last_updated INTEGER DEFAULT 0,
		name TEXT DEFAULT '',
		picture TEXT DEFAULT '',
		project_id TEXT DEFAULT ''
	);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_antigravity_quotas_email ON antigravity_quotas(email);

	CREATE TABLE IF NOT EXISTS api_tokens (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		token TEXT NOT NULL UNIQUE,
		token_prefix TEXT NOT NULL,
		name TEXT NOT NULL,
		description TEXT DEFAULT '',
		project_id INTEGER DEFAULT 0,
		is_enabled INTEGER DEFAULT 1,
		expires_at DATETIME,
		last_used_at DATETIME,
		use_count INTEGER DEFAULT 0,
		deleted_at DATETIME
	);
	CREATE INDEX IF NOT EXISTS idx_api_tokens_token ON api_tokens(token);

	CREATE TABLE IF NOT EXISTS model_mappings (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		client_type TEXT DEFAULT '',
		provider_id INTEGER DEFAULT 0,
		project_id INTEGER DEFAULT 0,
		route_id INTEGER DEFAULT 0,
		api_token_id INTEGER DEFAULT 0,
		pattern TEXT NOT NULL,
		target TEXT NOT NULL,
		priority INTEGER DEFAULT 0,
		is_enabled INTEGER DEFAULT 1,
		is_builtin INTEGER DEFAULT 0
	);
	CREATE INDEX IF NOT EXISTS idx_model_mappings_client_type ON model_mappings(client_type);
	CREATE INDEX IF NOT EXISTS idx_model_mappings_provider_id ON model_mappings(provider_id);
	CREATE INDEX IF NOT EXISTS idx_model_mappings_project_id ON model_mappings(project_id);
	CREATE INDEX IF NOT EXISTS idx_model_mappings_priority ON model_mappings(priority);

	CREATE TABLE IF NOT EXISTS usage_stats (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		hour DATETIME NOT NULL,
		route_id INTEGER DEFAULT 0,
		provider_id INTEGER DEFAULT 0,
		project_id INTEGER DEFAULT 0,
		api_token_id INTEGER DEFAULT 0,
		client_type TEXT DEFAULT '',
		total_requests INTEGER DEFAULT 0,
		successful_requests INTEGER DEFAULT 0,
		failed_requests INTEGER DEFAULT 0,
		input_tokens INTEGER DEFAULT 0,
		output_tokens INTEGER DEFAULT 0,
		cache_read INTEGER DEFAULT 0,
		cache_write INTEGER DEFAULT 0,
		cost INTEGER DEFAULT 0,
		UNIQUE(hour, route_id, provider_id, project_id, api_token_id, client_type)
	);
	CREATE INDEX IF NOT EXISTS idx_usage_stats_hour ON usage_stats(hour);
	CREATE INDEX IF NOT EXISTS idx_usage_stats_provider_id ON usage_stats(provider_id);
	CREATE INDEX IF NOT EXISTS idx_usage_stats_route_id ON usage_stats(route_id);
	CREATE INDEX IF NOT EXISTS idx_usage_stats_project_id ON usage_stats(project_id);
	CREATE INDEX IF NOT EXISTS idx_usage_stats_api_token_id ON usage_stats(api_token_id);
	`

	_, err := d.db.Exec(schema)
	if err != nil {
		return err
	}

	// Migration: Add reason column to cooldowns if it doesn't exist
	// Check if reason column exists
	var hasReason bool
	row := d.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('cooldowns') WHERE name='reason'`)
	row.Scan(&hasReason)

	if !hasReason {
		_, err = d.db.Exec(`ALTER TABLE cooldowns ADD COLUMN reason TEXT NOT NULL DEFAULT 'unknown'`)
		if err != nil {
			return err
		}
	}

	// Migration: Add slug column to projects if it doesn't exist
	var hasSlug bool
	row = d.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('projects') WHERE name='slug'`)
	row.Scan(&hasSlug)

	if !hasSlug {
		_, err = d.db.Exec(`ALTER TABLE projects ADD COLUMN slug TEXT NOT NULL DEFAULT ''`)
		if err != nil {
			return err
		}
	}

	// Create unique index for slug (must be after ALTER TABLE in case column was just added)
	_, _ = d.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_projects_slug ON projects(slug) WHERE slug != ''`)

	// Generate slugs for existing projects that don't have one
	if err := d.migrateProjectSlugs(); err != nil {
		return err
	}

	// Migration: Add project_id column to proxy_requests if it doesn't exist
	var hasProjectID bool
	row = d.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('proxy_requests') WHERE name='project_id'`)
	row.Scan(&hasProjectID)

	if !hasProjectID {
		_, err = d.db.Exec(`ALTER TABLE proxy_requests ADD COLUMN project_id INTEGER DEFAULT 0`)
		if err != nil {
			return err
		}
	}

	// Migration: Add enabled_custom_routes column to projects if it doesn't exist
	var hasEnabledCustomRoutes bool
	row = d.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('projects') WHERE name='enabled_custom_routes'`)
	row.Scan(&hasEnabledCustomRoutes)

	if !hasEnabledCustomRoutes {
		// Check if old column name exists
		var hasOldColumn bool
		row = d.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('projects') WHERE name='enabled_client_types'`)
		row.Scan(&hasOldColumn)

		if hasOldColumn {
			// Rename old column to new name
			_, err = d.db.Exec(`ALTER TABLE projects RENAME COLUMN enabled_client_types TO enabled_custom_routes`)
			if err != nil {
				return err
			}
		} else {
			// Create new column
			_, err = d.db.Exec(`ALTER TABLE projects ADD COLUMN enabled_custom_routes TEXT DEFAULT '[]'`)
			if err != nil {
				return err
			}
		}
	}

	// Migration: Add start_time, end_time and duration columns to proxy_upstream_attempts
	var hasStartTime bool
	row = d.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('proxy_upstream_attempts') WHERE name='start_time'`)
	row.Scan(&hasStartTime)

	if !hasStartTime {
		_, err = d.db.Exec(`ALTER TABLE proxy_upstream_attempts ADD COLUMN start_time DATETIME`)
		if err != nil {
			return err
		}
		_, err = d.db.Exec(`ALTER TABLE proxy_upstream_attempts ADD COLUMN end_time DATETIME`)
		if err != nil {
			return err
		}
		_, err = d.db.Exec(`ALTER TABLE proxy_upstream_attempts ADD COLUMN duration_ms INTEGER DEFAULT 0`)
		if err != nil {
			return err
		}
	}

	// Migration: Add status_code column to proxy_requests if it doesn't exist
	var hasStatusCode bool
	row = d.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('proxy_requests') WHERE name='status_code'`)
	row.Scan(&hasStatusCode)

	if !hasStatusCode {
		_, err = d.db.Exec(`ALTER TABLE proxy_requests ADD COLUMN status_code INTEGER DEFAULT 0`)
		if err != nil {
			return err
		}
	}

	// Migration: Add rejected_at column to sessions if it doesn't exist
	var hasRejectedAt bool
	row = d.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('sessions') WHERE name='rejected_at'`)
	row.Scan(&hasRejectedAt)

	if !hasRejectedAt {
		// Check if old 'rejected' column exists and migrate
		var hasOldRejected bool
		row = d.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('sessions') WHERE name='rejected'`)
		row.Scan(&hasOldRejected)

		// Add new column
		_, err = d.db.Exec(`ALTER TABLE sessions ADD COLUMN rejected_at DATETIME`)
		if err != nil {
			return err
		}

		// Migrate old data if exists (rejected=1 -> rejected_at=now)
		if hasOldRejected {
			_, _ = d.db.Exec(`UPDATE sessions SET rejected_at = CURRENT_TIMESTAMP WHERE rejected = 1`)
		}
	}

	// Migration: Add request_model and mapped_model columns to proxy_upstream_attempts
	var hasRequestModel bool
	row = d.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('proxy_upstream_attempts') WHERE name='request_model'`)
	row.Scan(&hasRequestModel)

	if !hasRequestModel {
		_, err = d.db.Exec(`ALTER TABLE proxy_upstream_attempts ADD COLUMN request_model TEXT DEFAULT ''`)
		if err != nil {
			return err
		}
		_, err = d.db.Exec(`ALTER TABLE proxy_upstream_attempts ADD COLUMN mapped_model TEXT DEFAULT ''`)
		if err != nil {
			return err
		}
	}

	// Migration: Add response_model column to proxy_upstream_attempts
	var hasResponseModel bool
	row = d.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('proxy_upstream_attempts') WHERE name='response_model'`)
	row.Scan(&hasResponseModel)

	if !hasResponseModel {
		_, err = d.db.Exec(`ALTER TABLE proxy_upstream_attempts ADD COLUMN response_model TEXT DEFAULT ''`)
		if err != nil {
			return err
		}
	}

	// Migration: Add api_token_id column to proxy_requests if it doesn't exist
	var hasAPITokenID bool
	row = d.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('proxy_requests') WHERE name='api_token_id'`)
	row.Scan(&hasAPITokenID)

	if !hasAPITokenID {
		_, err = d.db.Exec(`ALTER TABLE proxy_requests ADD COLUMN api_token_id INTEGER DEFAULT 0`)
		if err != nil {
			return err
		}
	}

	// Migration: Rebuild usage_stats table to fix UNIQUE constraint
	// Check if the table has the old UNIQUE constraint (without api_token_id)
	var hasOldConstraint bool
	row = d.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='sqlite_autoindex_usage_stats_1'`)
	row.Scan(&hasOldConstraint)

	if hasOldConstraint {
		// SQLite 不支持修改 UNIQUE 约束，需要重建表
		_, err = d.db.Exec(`
			-- 创建新表
			CREATE TABLE IF NOT EXISTS usage_stats_new (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				hour DATETIME NOT NULL,
				route_id INTEGER DEFAULT 0,
				provider_id INTEGER DEFAULT 0,
				project_id INTEGER DEFAULT 0,
				api_token_id INTEGER DEFAULT 0,
				client_type TEXT DEFAULT '',
				total_requests INTEGER DEFAULT 0,
				successful_requests INTEGER DEFAULT 0,
				failed_requests INTEGER DEFAULT 0,
				input_tokens INTEGER DEFAULT 0,
				output_tokens INTEGER DEFAULT 0,
				cache_read INTEGER DEFAULT 0,
				cache_write INTEGER DEFAULT 0,
				cost INTEGER DEFAULT 0,
				UNIQUE(hour, route_id, provider_id, project_id, api_token_id, client_type)
			);
			-- 迁移数据（清空旧数据，因为约束不兼容）
			DROP TABLE usage_stats;
			ALTER TABLE usage_stats_new RENAME TO usage_stats;
			-- 重建索引
			CREATE INDEX IF NOT EXISTS idx_usage_stats_hour ON usage_stats(hour);
			CREATE INDEX IF NOT EXISTS idx_usage_stats_provider_id ON usage_stats(provider_id);
			CREATE INDEX IF NOT EXISTS idx_usage_stats_route_id ON usage_stats(route_id);
			CREATE INDEX IF NOT EXISTS idx_usage_stats_project_id ON usage_stats(project_id);
			CREATE INDEX IF NOT EXISTS idx_usage_stats_api_token_id ON usage_stats(api_token_id);
		`)
		if err != nil {
			return fmt.Errorf("failed to rebuild usage_stats table: %w", err)
		}
	} else {
		// 检查是否缺少 api_token_id 列（更旧的数据库）
		var hasAPITokenIDCol bool
		row = d.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('usage_stats') WHERE name='api_token_id'`)
		row.Scan(&hasAPITokenIDCol)

		if !hasAPITokenIDCol {
			// 同样需要重建表
			_, err = d.db.Exec(`
				CREATE TABLE IF NOT EXISTS usage_stats_new (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					hour DATETIME NOT NULL,
					route_id INTEGER DEFAULT 0,
					provider_id INTEGER DEFAULT 0,
					project_id INTEGER DEFAULT 0,
					api_token_id INTEGER DEFAULT 0,
					client_type TEXT DEFAULT '',
					total_requests INTEGER DEFAULT 0,
					successful_requests INTEGER DEFAULT 0,
					failed_requests INTEGER DEFAULT 0,
					input_tokens INTEGER DEFAULT 0,
					output_tokens INTEGER DEFAULT 0,
					cache_read INTEGER DEFAULT 0,
					cache_write INTEGER DEFAULT 0,
					cost INTEGER DEFAULT 0,
					UNIQUE(hour, route_id, provider_id, project_id, api_token_id, client_type)
				);
				DROP TABLE usage_stats;
				ALTER TABLE usage_stats_new RENAME TO usage_stats;
				CREATE INDEX IF NOT EXISTS idx_usage_stats_hour ON usage_stats(hour);
				CREATE INDEX IF NOT EXISTS idx_usage_stats_provider_id ON usage_stats(provider_id);
				CREATE INDEX IF NOT EXISTS idx_usage_stats_route_id ON usage_stats(route_id);
				CREATE INDEX IF NOT EXISTS idx_usage_stats_project_id ON usage_stats(project_id);
				CREATE INDEX IF NOT EXISTS idx_usage_stats_api_token_id ON usage_stats(api_token_id);
			`)
			if err != nil {
				return fmt.Errorf("failed to rebuild usage_stats table: %w", err)
			}
		}
	}

	// Migration: Seed default model mappings if table is empty
	if err := d.seedDefaultModelMappings(); err != nil {
		return err
	}

	return nil
}

// migrateProjectSlugs generates slugs for existing projects that don't have one
func (d *DB) migrateProjectSlugs() error {
	// Get all projects without slugs
	rows, err := d.db.Query(`SELECT id, name FROM projects WHERE slug = ''`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type projectInfo struct {
		id   uint64
		name string
	}

	var projects []projectInfo
	for rows.Next() {
		var p projectInfo
		if err := rows.Scan(&p.id, &p.name); err != nil {
			return err
		}
		projects = append(projects, p)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	// Generate and update slugs
	usedSlugs := make(map[string]bool)

	// First, get all existing slugs
	existingRows, err := d.db.Query(`SELECT slug FROM projects WHERE slug != ''`)
	if err != nil {
		return err
	}
	defer existingRows.Close()

	for existingRows.Next() {
		var slug string
		if err := existingRows.Scan(&slug); err != nil {
			return err
		}
		usedSlugs[slug] = true
	}

	for _, p := range projects {
		baseSlug := generateSlug(p.name)
		slug := baseSlug
		counter := 1

		// Ensure uniqueness
		for usedSlugs[slug] {
			counter++
			slug = baseSlug + "-" + itoa(counter)
		}

		usedSlugs[slug] = true

		_, err := d.db.Exec(`UPDATE projects SET slug = ? WHERE id = ?`, slug, p.id)
		if err != nil {
			return err
		}
	}

	return nil
}

// generateSlug creates a URL-friendly slug from a name
func generateSlug(name string) string {
	// Convert to lowercase and replace non-alphanumeric with hyphens
	var result []byte
	lastWasHyphen := true // Start as true to avoid leading hyphen

	for i := 0; i < len(name); i++ {
		c := name[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			result = append(result, c)
			lastWasHyphen = false
		} else if c >= 'A' && c <= 'Z' {
			result = append(result, c+32) // Convert to lowercase
			lastWasHyphen = false
		} else if !lastWasHyphen {
			result = append(result, '-')
			lastWasHyphen = true
		}
	}

	// Remove trailing hyphen
	if len(result) > 0 && result[len(result)-1] == '-' {
		result = result[:len(result)-1]
	}

	if len(result) == 0 {
		return "project"
	}

	return string(result)
}

// itoa converts int to string without importing strconv
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var result []byte
	for i > 0 {
		result = append([]byte{byte('0' + i%10)}, result...)
		i /= 10
	}
	return string(result)
}

// Helper functions for JSON serialization
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

func parseTime(t sql.NullTime) time.Time {
	if t.Valid {
		return t.Time
	}
	return time.Time{}
}

// parseTimeString parses a string timestamp into time.Time
// Supports multiple formats due to SQLite driver behavior differences
func parseTimeString(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}

	// Try RFC3339 first (what go-sqlite3 returns)
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}

	// Try our storage format
	if t, err := time.Parse("2006-01-02 15:04:05", s); err == nil {
		return t, nil
	}

	return time.Time{}, nil
}

// formatTime formats a time.Time into a string for SQLite
func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02 15:04:05")
}

func nullTime(t time.Time) sql.NullTime {
	if t.IsZero() {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: t, Valid: true}
}

// seedDefaultModelMappings seeds the default model mappings if the table is empty
func (d *DB) seedDefaultModelMappings() error {
	// Check if table has any rows
	var count int
	if err := d.db.QueryRow(`SELECT COUNT(*) FROM model_mappings`).Scan(&count); err != nil {
		return err
	}

	// Only seed if table is empty
	if count > 0 {
		return nil
	}

	// Default mapping rules for Claude client type (Antigravity compatible)
	defaultRules := []struct {
		clientType string
		pattern    string
		target     string
		priority   int
	}{
		// OpenAI models -> Gemini (for claude client type)
		{"claude", "gpt-4o-mini*", "gemini-2.5-flash", 10},
		{"claude", "gpt-4o*", "gemini-3-flash", 20},
		{"claude", "gpt-4*", "gemini-3-pro-high", 30},
		{"claude", "gpt-3.5*", "gemini-2.5-flash", 40},
		{"claude", "o1-*", "gemini-3-pro-high", 50},
		{"claude", "o3-*", "gemini-3-pro-high", 60},

		// Claude models - specific patterns first
		{"claude", "claude-3-5-sonnet-*", "claude-sonnet-4-5", 100},
		{"claude", "claude-3-opus-*", "claude-opus-4-5-thinking", 110},
		{"claude", "claude-opus-4-*", "claude-opus-4-5-thinking", 120},
		{"claude", "claude-haiku-*", "gemini-2.5-flash-lite", 130},
		{"claude", "claude-3-haiku-*", "gemini-2.5-flash-lite", 140},

		// Generic Claude fallbacks (broad wildcards last)
		{"claude", "*opus*", "claude-opus-4-5-thinking", 200},
		{"claude", "*sonnet*", "claude-sonnet-4-5", 210},
		{"claude", "*haiku*", "gemini-2.5-flash-lite", 220},
	}

	for _, rule := range defaultRules {
		_, err := d.db.Exec(
			`INSERT INTO model_mappings (client_type, pattern, target, priority, is_enabled, is_builtin) VALUES (?, ?, ?, ?, 1, 1)`,
			rule.clientType, rule.pattern, rule.target, rule.priority,
		)
		if err != nil {
			return err
		}
	}

	return nil
}
