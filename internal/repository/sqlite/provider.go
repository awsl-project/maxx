package sqlite

import (
	"database/sql"
	"time"

	"github.com/Bowl42/maxx-next/internal/domain"
)

type ProviderRepository struct {
	db *DB
}

func NewProviderRepository(db *DB) *ProviderRepository {
	return &ProviderRepository{db: db}
}

func (r *ProviderRepository) Create(p *domain.Provider) error {
	now := time.Now()
	p.CreatedAt = now
	p.UpdatedAt = now

	result, err := r.db.db.Exec(
		`INSERT INTO providers (created_at, updated_at, type, name, config, supported_client_types) VALUES (?, ?, ?, ?, ?, ?)`,
		p.CreatedAt, p.UpdatedAt, p.Type, p.Name, toJSON(p.Config), toJSON(p.SupportedClientTypes),
	)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	p.ID = uint64(id)
	return nil
}

func (r *ProviderRepository) Update(p *domain.Provider) error {
	p.UpdatedAt = time.Now()
	_, err := r.db.db.Exec(
		`UPDATE providers SET updated_at = ?, type = ?, name = ?, config = ?, supported_client_types = ? WHERE id = ?`,
		p.UpdatedAt, p.Type, p.Name, toJSON(p.Config), toJSON(p.SupportedClientTypes), p.ID,
	)
	return err
}

func (r *ProviderRepository) Delete(id uint64) error {
	now := time.Now()
	_, err := r.db.db.Exec(`UPDATE providers SET deleted_at = ?, updated_at = ? WHERE id = ?`, now, now, id)
	return err
}

func (r *ProviderRepository) GetByID(id uint64) (*domain.Provider, error) {
	row := r.db.db.QueryRow(`SELECT id, created_at, updated_at, deleted_at, type, name, config, supported_client_types FROM providers WHERE id = ?`, id)
	return r.scanProvider(row)
}

func (r *ProviderRepository) List() ([]*domain.Provider, error) {
	rows, err := r.db.db.Query(`SELECT id, created_at, updated_at, deleted_at, type, name, config, supported_client_types FROM providers WHERE deleted_at IS NULL ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var providers []*domain.Provider
	for rows.Next() {
		p, err := r.scanProviderRows(rows)
		if err != nil {
			return nil, err
		}
		providers = append(providers, p)
	}
	return providers, rows.Err()
}

func (r *ProviderRepository) scanProvider(row *sql.Row) (*domain.Provider, error) {
	var p domain.Provider
	var configJSON, typesJSON string
	var deletedAt sql.NullTime
	err := row.Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt, &deletedAt, &p.Type, &p.Name, &configJSON, &typesJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	if deletedAt.Valid {
		p.DeletedAt = &deletedAt.Time
	}
	p.Config = fromJSON[*domain.ProviderConfig](configJSON)
	p.SupportedClientTypes = fromJSON[[]domain.ClientType](typesJSON)
	return &p, nil
}

func (r *ProviderRepository) scanProviderRows(rows *sql.Rows) (*domain.Provider, error) {
	var p domain.Provider
	var configJSON, typesJSON string
	var deletedAt sql.NullTime
	err := rows.Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt, &deletedAt, &p.Type, &p.Name, &configJSON, &typesJSON)
	if err != nil {
		return nil, err
	}
	if deletedAt.Valid {
		p.DeletedAt = &deletedAt.Time
	}
	p.Config = fromJSON[*domain.ProviderConfig](configJSON)
	p.SupportedClientTypes = fromJSON[[]domain.ClientType](typesJSON)
	return &p, nil
}
