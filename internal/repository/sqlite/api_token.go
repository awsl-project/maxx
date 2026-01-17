package sqlite

import (
	"database/sql"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

type APITokenRepository struct {
	db *DB
}

func NewAPITokenRepository(db *DB) *APITokenRepository {
	return &APITokenRepository{db: db}
}

func (r *APITokenRepository) Create(t *domain.APIToken) error {
	now := time.Now()
	t.CreatedAt = now
	t.UpdatedAt = now

	result, err := r.db.db.Exec(
		`INSERT INTO api_tokens (created_at, updated_at, token, token_prefix, name, description, project_id, is_enabled, expires_at, last_used_at, use_count) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.CreatedAt, t.UpdatedAt, t.Token, t.TokenPrefix, t.Name, t.Description, t.ProjectID, t.IsEnabled, formatTimePtr(t.ExpiresAt), formatTimePtr(t.LastUsedAt), t.UseCount,
	)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	t.ID = uint64(id)
	return nil
}

func (r *APITokenRepository) Update(t *domain.APIToken) error {
	t.UpdatedAt = time.Now()
	_, err := r.db.db.Exec(
		`UPDATE api_tokens SET updated_at = ?, name = ?, description = ?, project_id = ?, is_enabled = ?, expires_at = ? WHERE id = ?`,
		t.UpdatedAt, t.Name, t.Description, t.ProjectID, t.IsEnabled, formatTimePtr(t.ExpiresAt), t.ID,
	)
	return err
}

func (r *APITokenRepository) Delete(id uint64) error {
	_, err := r.db.db.Exec(`UPDATE api_tokens SET deleted_at = ?, updated_at = ? WHERE id = ?`, formatTime(time.Now()), formatTime(time.Now()), id)
	return err
}

func (r *APITokenRepository) GetByID(id uint64) (*domain.APIToken, error) {
	row := r.db.db.QueryRow(`SELECT id, created_at, updated_at, token, token_prefix, name, description, project_id, is_enabled, expires_at, last_used_at, use_count, deleted_at FROM api_tokens WHERE id = ?`, id)
	return r.scanToken(row)
}

func (r *APITokenRepository) GetByToken(token string) (*domain.APIToken, error) {
	row := r.db.db.QueryRow(`SELECT id, created_at, updated_at, token, token_prefix, name, description, project_id, is_enabled, expires_at, last_used_at, use_count, deleted_at FROM api_tokens WHERE token = ? AND deleted_at IS NULL`, token)
	return r.scanToken(row)
}

func (r *APITokenRepository) List() ([]*domain.APIToken, error) {
	rows, err := r.db.db.Query(`SELECT id, created_at, updated_at, token, token_prefix, name, description, project_id, is_enabled, expires_at, last_used_at, use_count, deleted_at FROM api_tokens WHERE deleted_at IS NULL ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tokens := make([]*domain.APIToken, 0)
	for rows.Next() {
		t, err := r.scanTokenRow(rows)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}

func (r *APITokenRepository) IncrementUseCount(id uint64) error {
	_, err := r.db.db.Exec(
		`UPDATE api_tokens SET use_count = use_count + 1, last_used_at = ?, updated_at = ? WHERE id = ?`,
		formatTime(time.Now()), formatTime(time.Now()), id,
	)
	return err
}

func (r *APITokenRepository) scanToken(row *sql.Row) (*domain.APIToken, error) {
	var t domain.APIToken
	var expiresAt, lastUsedAt, deletedAt sql.NullString

	err := row.Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt, &t.Token, &t.TokenPrefix, &t.Name, &t.Description, &t.ProjectID, &t.IsEnabled, &expiresAt, &lastUsedAt, &t.UseCount, &deletedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}

	if expiresAt.Valid && expiresAt.String != "" {
		if parsed, err := parseTimeString(expiresAt.String); err == nil && !parsed.IsZero() {
			t.ExpiresAt = &parsed
		}
	}
	if lastUsedAt.Valid && lastUsedAt.String != "" {
		if parsed, err := parseTimeString(lastUsedAt.String); err == nil && !parsed.IsZero() {
			t.LastUsedAt = &parsed
		}
	}
	if deletedAt.Valid && deletedAt.String != "" {
		if parsed, err := parseTimeString(deletedAt.String); err == nil && !parsed.IsZero() {
			t.DeletedAt = &parsed
		}
	}

	return &t, nil
}

func (r *APITokenRepository) scanTokenRow(rows *sql.Rows) (*domain.APIToken, error) {
	var t domain.APIToken
	var expiresAt, lastUsedAt, deletedAt sql.NullString

	err := rows.Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt, &t.Token, &t.TokenPrefix, &t.Name, &t.Description, &t.ProjectID, &t.IsEnabled, &expiresAt, &lastUsedAt, &t.UseCount, &deletedAt)
	if err != nil {
		return nil, err
	}

	if expiresAt.Valid && expiresAt.String != "" {
		if parsed, err := parseTimeString(expiresAt.String); err == nil && !parsed.IsZero() {
			t.ExpiresAt = &parsed
		}
	}
	if lastUsedAt.Valid && lastUsedAt.String != "" {
		if parsed, err := parseTimeString(lastUsedAt.String); err == nil && !parsed.IsZero() {
			t.LastUsedAt = &parsed
		}
	}
	if deletedAt.Valid && deletedAt.String != "" {
		if parsed, err := parseTimeString(deletedAt.String); err == nil && !parsed.IsZero() {
			t.DeletedAt = &parsed
		}
	}

	return &t, nil
}
