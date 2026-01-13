package sqlite

import (
	"database/sql"
	"time"

	"github.com/Bowl42/maxx-next/internal/domain"
	"github.com/Bowl42/maxx-next/internal/repository"
)

type CooldownRepository struct {
	db *DB
}

func NewCooldownRepository(db *DB) repository.CooldownRepository {
	return &CooldownRepository{db: db}
}

func (r *CooldownRepository) GetAll() ([]*domain.Cooldown, error) {
	query := `SELECT id, created_at, updated_at, provider_id, client_type, until_time, reason
	          FROM cooldowns
	          WHERE until_time > datetime('now')`

	rows, err := r.db.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cooldowns []*domain.Cooldown
	for rows.Next() {
		cd := &domain.Cooldown{}
		var createdAt, updatedAt, untilTime string
		var reason string
		if err := rows.Scan(&cd.ID, &createdAt, &updatedAt, &cd.ProviderID, &cd.ClientType, &untilTime, &reason); err != nil {
			return nil, err
		}

		cd.CreatedAt, _ = parseTimeString(createdAt)
		cd.UpdatedAt, _ = parseTimeString(updatedAt)
		cd.UntilTime, _ = parseTimeString(untilTime)
		cd.Reason = domain.CooldownReason(reason)
		cooldowns = append(cooldowns, cd)
	}

	return cooldowns, rows.Err()
}

func (r *CooldownRepository) GetByProvider(providerID uint64) ([]*domain.Cooldown, error) {
	query := `SELECT id, created_at, updated_at, provider_id, client_type, until_time, reason
	          FROM cooldowns
	          WHERE provider_id = ? AND until_time > datetime('now')`

	rows, err := r.db.db.Query(query, providerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cooldowns []*domain.Cooldown
	for rows.Next() {
		cd := &domain.Cooldown{}
		var createdAt, updatedAt, untilTime string
		var reason string
		if err := rows.Scan(&cd.ID, &createdAt, &updatedAt, &cd.ProviderID, &cd.ClientType, &untilTime, &reason); err != nil {
			return nil, err
		}

		cd.CreatedAt, _ = parseTimeString(createdAt)
		cd.UpdatedAt, _ = parseTimeString(updatedAt)
		cd.UntilTime, _ = parseTimeString(untilTime)
		cd.Reason = domain.CooldownReason(reason)
		cooldowns = append(cooldowns, cd)
	}

	return cooldowns, rows.Err()
}

func (r *CooldownRepository) Get(providerID uint64, clientType string) (*domain.Cooldown, error) {
	query := `SELECT id, created_at, updated_at, provider_id, client_type, until_time, reason
	          FROM cooldowns
	          WHERE provider_id = ? AND client_type = ? AND until_time > datetime('now')`

	cd := &domain.Cooldown{}
	var createdAt, updatedAt, untilTime string
	var reason string

	err := r.db.db.QueryRow(query, providerID, clientType).Scan(
		&cd.ID, &createdAt, &updatedAt, &cd.ProviderID, &cd.ClientType, &untilTime, &reason,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	cd.CreatedAt, _ = parseTimeString(createdAt)
	cd.UpdatedAt, _ = parseTimeString(updatedAt)
	cd.UntilTime, _ = parseTimeString(untilTime)
	cd.Reason = domain.CooldownReason(reason)

	return cd, nil
}

func (r *CooldownRepository) Upsert(cooldown *domain.Cooldown) error {
	now := time.Now().UTC()

	// Use INSERT OR REPLACE to handle both insert and update cases
	// This works correctly even when an expired record exists
	query := `INSERT INTO cooldowns (provider_id, client_type, until_time, reason, created_at, updated_at)
	          VALUES (?, ?, ?, ?, ?, ?)
	          ON CONFLICT(provider_id, client_type) DO UPDATE SET
	            until_time = excluded.until_time,
	            reason = excluded.reason,
	            updated_at = excluded.updated_at`

	_, err := r.db.db.Exec(query,
		cooldown.ProviderID,
		cooldown.ClientType,
		formatTime(cooldown.UntilTime),
		string(cooldown.Reason),
		formatTime(now),
		formatTime(now),
	)

	if err != nil {
		return err
	}

	cooldown.CreatedAt = now
	cooldown.UpdatedAt = now

	return nil
}

func (r *CooldownRepository) Delete(providerID uint64, clientType string) error {
	query := `DELETE FROM cooldowns WHERE provider_id = ? AND client_type = ?`
	_, err := r.db.db.Exec(query, providerID, clientType)
	return err
}

func (r *CooldownRepository) DeleteAll(providerID uint64) error {
	query := `DELETE FROM cooldowns WHERE provider_id = ?`
	_, err := r.db.db.Exec(query, providerID)
	return err
}

func (r *CooldownRepository) DeleteExpired() error {
	query := `DELETE FROM cooldowns WHERE until_time <= datetime('now')`
	_, err := r.db.db.Exec(query)
	return err
}
