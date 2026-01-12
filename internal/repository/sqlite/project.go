package sqlite

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/Bowl42/maxx-next/internal/domain"
)

type ProjectRepository struct {
	db *DB
}

func NewProjectRepository(db *DB) *ProjectRepository {
	return &ProjectRepository{db: db}
}

func (r *ProjectRepository) Create(p *domain.Project) error {
	now := time.Now()
	p.CreatedAt = now
	p.UpdatedAt = now

	// Generate slug if not provided
	if p.Slug == "" {
		p.Slug = domain.GenerateSlug(p.Name)
	}

	// Ensure slug uniqueness
	baseSlug := p.Slug
	counter := 1
	for {
		var exists bool
		err := r.db.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM projects WHERE slug = ?)`, p.Slug).Scan(&exists)
		if err != nil {
			return err
		}
		if !exists {
			break
		}
		counter++
		p.Slug = baseSlug + "-" + itoa(counter)
	}

	// Serialize EnabledCustomRoutes
	enabledCustomRoutesJSON, err := json.Marshal(p.EnabledCustomRoutes)
	if err != nil {
		return err
	}

	result, err := r.db.db.Exec(
		`INSERT INTO projects (created_at, updated_at, name, slug, enabled_custom_routes) VALUES (?, ?, ?, ?, ?)`,
		p.CreatedAt, p.UpdatedAt, p.Name, p.Slug, string(enabledCustomRoutesJSON),
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

func (r *ProjectRepository) Update(p *domain.Project) error {
	p.UpdatedAt = time.Now()

	// Check slug uniqueness (excluding current project)
	if p.Slug != "" {
		var exists bool
		err := r.db.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM projects WHERE slug = ? AND id != ?)`, p.Slug, p.ID).Scan(&exists)
		if err != nil {
			return err
		}
		if exists {
			return domain.ErrSlugExists
		}
	}

	// Serialize EnabledCustomRoutes
	enabledCustomRoutesJSON, err := json.Marshal(p.EnabledCustomRoutes)
	if err != nil {
		return err
	}

	_, err = r.db.db.Exec(
		`UPDATE projects SET updated_at = ?, name = ?, slug = ?, enabled_custom_routes = ? WHERE id = ?`,
		p.UpdatedAt, p.Name, p.Slug, string(enabledCustomRoutesJSON), p.ID,
	)
	return err
}

func (r *ProjectRepository) Delete(id uint64) error {
	_, err := r.db.db.Exec(`DELETE FROM projects WHERE id = ?`, id)
	return err
}

func (r *ProjectRepository) GetByID(id uint64) (*domain.Project, error) {
	row := r.db.db.QueryRow(`SELECT id, created_at, updated_at, name, slug, enabled_custom_routes FROM projects WHERE id = ?`, id)
	var p domain.Project
	var enabledCustomRoutesJSON string
	err := row.Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt, &p.Name, &p.Slug, &enabledCustomRoutesJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}

	// Deserialize EnabledCustomRoutes
	if err := json.Unmarshal([]byte(enabledCustomRoutesJSON), &p.EnabledCustomRoutes); err != nil {
		p.EnabledCustomRoutes = []domain.ClientType{} // Default to empty array on error
	}

	return &p, nil
}

func (r *ProjectRepository) GetBySlug(slug string) (*domain.Project, error) {
	row := r.db.db.QueryRow(`SELECT id, created_at, updated_at, name, slug, enabled_custom_routes FROM projects WHERE slug = ?`, slug)
	var p domain.Project
	var enabledCustomRoutesJSON string
	err := row.Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt, &p.Name, &p.Slug, &enabledCustomRoutesJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}

	// Deserialize EnabledCustomRoutes
	if err := json.Unmarshal([]byte(enabledCustomRoutesJSON), &p.EnabledCustomRoutes); err != nil {
		p.EnabledCustomRoutes = []domain.ClientType{} // Default to empty array on error
	}

	return &p, nil
}

func (r *ProjectRepository) List() ([]*domain.Project, error) {
	rows, err := r.db.db.Query(`SELECT id, created_at, updated_at, name, slug, enabled_custom_routes FROM projects ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []*domain.Project
	for rows.Next() {
		var p domain.Project
		var enabledCustomRoutesJSON string
		err := rows.Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt, &p.Name, &p.Slug, &enabledCustomRoutesJSON)
		if err != nil {
			return nil, err
		}

		// Deserialize EnabledCustomRoutes
		if err := json.Unmarshal([]byte(enabledCustomRoutesJSON), &p.EnabledCustomRoutes); err != nil {
			p.EnabledCustomRoutes = []domain.ClientType{} // Default to empty array on error
		}

		projects = append(projects, &p)
	}
	return projects, rows.Err()
}
