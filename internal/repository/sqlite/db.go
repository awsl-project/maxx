package sqlite

import (
	"database/sql"
	"encoding/json"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	db *sql.DB
}

func NewDB(path string) (*DB, error) {
	db, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_busy_timeout=5000")
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
		slug TEXT NOT NULL DEFAULT ''
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
