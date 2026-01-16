package sqlite

import (
	"database/sql"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

type RouteRepository struct {
	db *DB
}

func NewRouteRepository(db *DB) *RouteRepository {
	return &RouteRepository{db: db}
}

func (r *RouteRepository) Create(route *domain.Route) error {
	now := time.Now()
	route.CreatedAt = now
	route.UpdatedAt = now

	isEnabled := 0
	if route.IsEnabled {
		isEnabled = 1
	}
	isNative := 0
	if route.IsNative {
		isNative = 1
	}

	result, err := r.db.db.Exec(
		`INSERT INTO routes (created_at, updated_at, is_enabled, is_native, project_id, client_type, provider_id, position, retry_config_id, model_mapping) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		route.CreatedAt, route.UpdatedAt, isEnabled, isNative, route.ProjectID, route.ClientType, route.ProviderID, route.Position, route.RetryConfigID, toJSON(route.ModelMapping),
	)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	route.ID = uint64(id)
	return nil
}

func (r *RouteRepository) Update(route *domain.Route) error {
	route.UpdatedAt = time.Now()
	isEnabled := 0
	if route.IsEnabled {
		isEnabled = 1
	}
	isNative := 0
	if route.IsNative {
		isNative = 1
	}
	_, err := r.db.db.Exec(
		`UPDATE routes SET updated_at = ?, is_enabled = ?, is_native = ?, project_id = ?, client_type = ?, provider_id = ?, position = ?, retry_config_id = ?, model_mapping = ? WHERE id = ?`,
		route.UpdatedAt, isEnabled, isNative, route.ProjectID, route.ClientType, route.ProviderID, route.Position, route.RetryConfigID, toJSON(route.ModelMapping), route.ID,
	)
	return err
}

	func (r *RouteRepository) Delete(id uint64) error {
	_, err := r.db.db.Exec(`DELETE FROM routes WHERE id = ?`, id)
	return err
}

func (r *RouteRepository) BatchUpdatePositions(updates []domain.RoutePositionUpdate) error {
	if len(updates) == 0 {
		return nil
	}

	tx, err := r.db.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now()
	stmt, err := tx.Prepare(`UPDATE routes SET position = ?, updated_at = ? WHERE id = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, update := range updates {
		if _, err := stmt.Exec(update.Position, now, update.ID); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *RouteRepository) GetByID(id uint64) (*domain.Route, error) {
	row := r.db.db.QueryRow(`SELECT id, created_at, updated_at, is_enabled, is_native, project_id, client_type, provider_id, position, retry_config_id, model_mapping FROM routes WHERE id = ?`, id)
	return r.scanRoute(row)
}

func (r *RouteRepository) FindByKey(projectID, providerID uint64, clientType domain.ClientType) (*domain.Route, error) {
	row := r.db.db.QueryRow(`SELECT id, created_at, updated_at, is_enabled, is_native, project_id, client_type, provider_id, position, retry_config_id, model_mapping FROM routes WHERE project_id = ? AND provider_id = ? AND client_type = ?`, projectID, providerID, clientType)
	return r.scanRoute(row)
}

func (r *RouteRepository) List() ([]*domain.Route, error) {
	rows, err := r.db.db.Query(`SELECT id, created_at, updated_at, is_enabled, is_native, project_id, client_type, provider_id, position, retry_config_id, model_mapping FROM routes ORDER BY position`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	routes := make([]*domain.Route, 0)
	for rows.Next() {
		route, err := r.scanRouteRows(rows)
		if err != nil {
			return nil, err
		}
		routes = append(routes, route)
	}
	return routes, rows.Err()
}

func (r *RouteRepository) scanRoute(row *sql.Row) (*domain.Route, error) {
	var route domain.Route
	var isEnabled, isNative int
	var mappingJSON string
	err := row.Scan(&route.ID, &route.CreatedAt, &route.UpdatedAt, &isEnabled, &isNative, &route.ProjectID, &route.ClientType, &route.ProviderID, &route.Position, &route.RetryConfigID, &mappingJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	route.IsEnabled = isEnabled == 1
	route.IsNative = isNative == 1
	route.ModelMapping = fromJSON[map[string]string](mappingJSON)
	return &route, nil
}

func (r *RouteRepository) scanRouteRows(rows *sql.Rows) (*domain.Route, error) {
	var route domain.Route
	var isEnabled, isNative int
	var mappingJSON string
	err := rows.Scan(&route.ID, &route.CreatedAt, &route.UpdatedAt, &isEnabled, &isNative, &route.ProjectID, &route.ClientType, &route.ProviderID, &route.Position, &route.RetryConfigID, &mappingJSON)
	if err != nil {
		return nil, err
	}
	route.IsEnabled = isEnabled == 1
	route.IsNative = isNative == 1
	route.ModelMapping = fromJSON[map[string]string](mappingJSON)
	return &route, nil
}
