package sqlite

import (
	"database/sql"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

type ModelMappingRepository struct {
	db *DB
}

func NewModelMappingRepository(db *DB) *ModelMappingRepository {
	return &ModelMappingRepository{db: db}
}

func (r *ModelMappingRepository) Create(mapping *domain.ModelMapping) error {
	now := time.Now()
	mapping.CreatedAt = now
	mapping.UpdatedAt = now

	isEnabled := 0
	if mapping.IsEnabled {
		isEnabled = 1
	}
	isBuiltin := 0
	if mapping.IsBuiltin {
		isBuiltin = 1
	}

	result, err := r.db.db.Exec(
		`INSERT INTO model_mappings (created_at, updated_at, client_type, provider_type, provider_id, project_id, route_id, api_token_id, pattern, target, priority, is_enabled, is_builtin) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		formatTime(mapping.CreatedAt), formatTime(mapping.UpdatedAt),
		mapping.ClientType, mapping.ProviderType, mapping.ProviderID, mapping.ProjectID, mapping.RouteID, mapping.APITokenID,
		mapping.Pattern, mapping.Target, mapping.Priority, isEnabled, isBuiltin,
	)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	mapping.ID = uint64(id)
	return nil
}

func (r *ModelMappingRepository) Update(mapping *domain.ModelMapping) error {
	mapping.UpdatedAt = time.Now()

	isEnabled := 0
	if mapping.IsEnabled {
		isEnabled = 1
	}
	isBuiltin := 0
	if mapping.IsBuiltin {
		isBuiltin = 1
	}

	_, err := r.db.db.Exec(
		`UPDATE model_mappings SET updated_at = ?, client_type = ?, provider_type = ?, provider_id = ?, project_id = ?, route_id = ?, api_token_id = ?, pattern = ?, target = ?, priority = ?, is_enabled = ?, is_builtin = ? WHERE id = ?`,
		formatTime(mapping.UpdatedAt),
		mapping.ClientType, mapping.ProviderType, mapping.ProviderID, mapping.ProjectID, mapping.RouteID, mapping.APITokenID,
		mapping.Pattern, mapping.Target, mapping.Priority, isEnabled, isBuiltin, mapping.ID,
	)
	return err
}

func (r *ModelMappingRepository) Delete(id uint64) error {
	_, err := r.db.db.Exec(`DELETE FROM model_mappings WHERE id = ? AND is_builtin = 0`, id)
	return err
}

func (r *ModelMappingRepository) GetByID(id uint64) (*domain.ModelMapping, error) {
	row := r.db.db.QueryRow(`SELECT id, created_at, updated_at, client_type, provider_type, provider_id, project_id, route_id, api_token_id, pattern, target, priority, is_enabled, is_builtin FROM model_mappings WHERE id = ?`, id)
	return r.scanMapping(row)
}

func (r *ModelMappingRepository) List() ([]*domain.ModelMapping, error) {
	rows, err := r.db.db.Query(`SELECT id, created_at, updated_at, client_type, provider_type, provider_id, project_id, route_id, api_token_id, pattern, target, priority, is_enabled, is_builtin FROM model_mappings ORDER BY priority, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanMappings(rows)
}

// ListEnabled returns all enabled mappings ordered by priority
func (r *ModelMappingRepository) ListEnabled() ([]*domain.ModelMapping, error) {
	rows, err := r.db.db.Query(`SELECT id, created_at, updated_at, client_type, provider_type, provider_id, project_id, route_id, api_token_id, pattern, target, priority, is_enabled, is_builtin FROM model_mappings WHERE is_enabled = 1 ORDER BY priority, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanMappings(rows)
}

// ListByQuery returns all enabled mappings matching the query conditions
// Matches rules where: (field is 0/empty OR field matches query)
func (r *ModelMappingRepository) ListByQuery(query *domain.ModelMappingQuery) ([]*domain.ModelMapping, error) {
	rows, err := r.db.db.Query(
		`SELECT id, created_at, updated_at, client_type, provider_type, provider_id, project_id, route_id, api_token_id, pattern, target, priority, is_enabled, is_builtin
		FROM model_mappings
		WHERE is_enabled = 1
		  AND (client_type = '' OR client_type = ?)
		  AND (provider_type = '' OR provider_type = ?)
		  AND (provider_id = 0 OR provider_id = ?)
		  AND (project_id = 0 OR project_id = ?)
		  AND (route_id = 0 OR route_id = ?)
		  AND (api_token_id = 0 OR api_token_id = ?)
		ORDER BY priority, id`,
		query.ClientType, query.ProviderType, query.ProviderID, query.ProjectID, query.RouteID, query.APITokenID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanMappings(rows)
}

// ListByClientType returns all enabled mappings for a specific client type (including global mappings)
func (r *ModelMappingRepository) ListByClientType(clientType domain.ClientType) ([]*domain.ModelMapping, error) {
	rows, err := r.db.db.Query(
		`SELECT id, created_at, updated_at, client_type, provider_type, provider_id, project_id, route_id, api_token_id, pattern, target, priority, is_enabled, is_builtin
		FROM model_mappings
		WHERE is_enabled = 1 AND (client_type = '' OR client_type = ?)
		ORDER BY priority, id`,
		clientType,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanMappings(rows)
}

// Count returns the total number of mappings
func (r *ModelMappingRepository) Count() (int, error) {
	var count int
	err := r.db.db.QueryRow(`SELECT COUNT(*) FROM model_mappings`).Scan(&count)
	return count, err
}

// DeleteAll deletes all non-builtin mappings
func (r *ModelMappingRepository) DeleteAll() error {
	_, err := r.db.db.Exec(`DELETE FROM model_mappings WHERE is_builtin = 0`)
	return err
}

// DeleteBuiltin deletes all builtin mappings (used for re-seeding)
func (r *ModelMappingRepository) DeleteBuiltin() error {
	_, err := r.db.db.Exec(`DELETE FROM model_mappings WHERE is_builtin = 1`)
	return err
}

// ClearAll deletes all mappings (both builtin and non-builtin)
func (r *ModelMappingRepository) ClearAll() error {
	_, err := r.db.db.Exec(`DELETE FROM model_mappings`)
	return err
}

// SeedDefaults deletes all builtin mappings and re-seeds with defaults
func (r *ModelMappingRepository) SeedDefaults() error {
	// Delete existing builtin mappings
	_, err := r.db.db.Exec(`DELETE FROM model_mappings WHERE is_builtin = 1`)
	if err != nil {
		return err
	}

	defaultRules := []struct {
		clientType   string
		providerType string
		pattern      string
		target       string
		priority     int
	}{
		{"claude", "antigravity", "gpt-4o-mini*", "gemini-2.5-flash", 10},
		{"claude", "antigravity", "gpt-4o*", "gemini-3-flash", 20},
		{"claude", "antigravity", "gpt-4*", "gemini-3-pro-high", 30},
		{"claude", "antigravity", "gpt-3.5*", "gemini-2.5-flash", 40},
		{"claude", "antigravity", "o1-*", "gemini-3-pro-high", 50},
		{"claude", "antigravity", "o3-*", "gemini-3-pro-high", 60},
		{"claude", "antigravity", "claude-3-5-sonnet-*", "claude-sonnet-4-5", 100},
		{"claude", "antigravity", "claude-3-opus-*", "claude-opus-4-5-thinking", 110},
		{"claude", "antigravity", "claude-opus-4-*", "claude-opus-4-5-thinking", 120},
		{"claude", "antigravity", "claude-haiku-*", "gemini-2.5-flash-lite", 130},
		{"claude", "antigravity", "claude-3-haiku-*", "gemini-2.5-flash-lite", 140},
		{"claude", "antigravity", "*opus*", "claude-opus-4-5-thinking", 200},
		{"claude", "antigravity", "*sonnet*", "claude-sonnet-4-5", 210},
		{"claude", "antigravity", "*haiku*", "gemini-2.5-flash-lite", 220},
	}

	for _, rule := range defaultRules {
		_, err := r.db.db.Exec(
			`INSERT INTO model_mappings (client_type, provider_type, pattern, target, priority, is_enabled, is_builtin) VALUES (?, ?, ?, ?, ?, 1, 1)`,
			rule.clientType, rule.providerType, rule.pattern, rule.target, rule.priority,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *ModelMappingRepository) scanMapping(row *sql.Row) (*domain.ModelMapping, error) {
	var mapping domain.ModelMapping
	var isEnabled, isBuiltin int
	var createdAt, updatedAt string

	err := row.Scan(&mapping.ID, &createdAt, &updatedAt,
		&mapping.ClientType, &mapping.ProviderType, &mapping.ProviderID, &mapping.ProjectID, &mapping.RouteID, &mapping.APITokenID,
		&mapping.Pattern, &mapping.Target, &mapping.Priority, &isEnabled, &isBuiltin)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}

	mapping.CreatedAt, _ = parseTimeString(createdAt)
	mapping.UpdatedAt, _ = parseTimeString(updatedAt)
	mapping.IsEnabled = isEnabled == 1
	mapping.IsBuiltin = isBuiltin == 1
	return &mapping, nil
}

func (r *ModelMappingRepository) scanMappings(rows *sql.Rows) ([]*domain.ModelMapping, error) {
	mappings := make([]*domain.ModelMapping, 0)
	for rows.Next() {
		var mapping domain.ModelMapping
		var isEnabled, isBuiltin int
		var createdAt, updatedAt string

		err := rows.Scan(&mapping.ID, &createdAt, &updatedAt,
			&mapping.ClientType, &mapping.ProviderType, &mapping.ProviderID, &mapping.ProjectID, &mapping.RouteID, &mapping.APITokenID,
			&mapping.Pattern, &mapping.Target, &mapping.Priority, &isEnabled, &isBuiltin)
		if err != nil {
			return nil, err
		}

		mapping.CreatedAt, _ = parseTimeString(createdAt)
		mapping.UpdatedAt, _ = parseTimeString(updatedAt)
		mapping.IsEnabled = isEnabled == 1
		mapping.IsBuiltin = isBuiltin == 1
		mappings = append(mappings, &mapping)
	}
	return mappings, rows.Err()
}
