package sqlite

import (
	"database/sql"
	"encoding/json"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	db *sql.DB
}

func NewDB(path string) (*DB, error) {
	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000")
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
		supported_client_types TEXT
	);

	CREATE TABLE IF NOT EXISTS projects (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		name TEXT NOT NULL
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
		cost INTEGER DEFAULT 0
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
		cost INTEGER DEFAULT 0
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
	`

	_, err := d.db.Exec(schema)
	if err != nil {
		return err
	}

	// Migration: add instance_id column if not exists
	d.db.Exec("ALTER TABLE proxy_requests ADD COLUMN instance_id TEXT")

	// Migration: add cache TTL columns for detailed cache tracking
	d.db.Exec("ALTER TABLE proxy_requests ADD COLUMN cache_5m_write_count INTEGER DEFAULT 0")
	d.db.Exec("ALTER TABLE proxy_requests ADD COLUMN cache_1h_write_count INTEGER DEFAULT 0")
	d.db.Exec("ALTER TABLE proxy_upstream_attempts ADD COLUMN cache_5m_write_count INTEGER DEFAULT 0")
	d.db.Exec("ALTER TABLE proxy_upstream_attempts ADD COLUMN cache_1h_write_count INTEGER DEFAULT 0")

	return nil
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

func nullTime(t time.Time) sql.NullTime {
	if t.IsZero() {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: t, Valid: true}
}
