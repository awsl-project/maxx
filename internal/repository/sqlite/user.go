package sqlite

import (
	"errors"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"gorm.io/gorm"
)

type UserRepository struct {
	db *DB
}

func NewUserRepository(db *DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Create(u *domain.User) error {
	now := time.Now()
	u.CreatedAt = now
	u.UpdatedAt = now

	model := r.toModel(u)
	if err := r.db.gorm.Create(model).Error; err != nil {
		return err
	}
	u.ID = model.ID
	return nil
}

func (r *UserRepository) Update(u *domain.User) error {
	u.UpdatedAt = time.Now()
	model := r.toModel(u)
	return r.db.gorm.Save(model).Error
}

// Delete 硬删除用户。用户表无业务数据关联，软删除会导致 username 唯一约束冲突，无法重新注册同名用户。
func (r *UserRepository) Delete(tenantID uint64, id uint64) error {
	result := r.db.gorm.Unscoped().Where("id = ? AND tenant_id = ?", id, tenantID).Delete(&User{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *UserRepository) GetByID(tenantID uint64, id uint64) (*domain.User, error) {
	var model User
	query := r.db.gorm.Where("deleted_at = 0")
	if tenantID > 0 {
		query = query.Where("tenant_id = ?", tenantID)
	}
	if err := query.First(&model, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return r.toDomain(&model), nil
}

func (r *UserRepository) GetByUsername(username string) (*domain.User, error) {
	var model User
	if err := r.db.gorm.Where("username = ? AND deleted_at = 0", username).First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return r.toDomain(&model), nil
}

func (r *UserRepository) GetDefault() (*domain.User, error) {
	var model User
	if err := r.db.gorm.Where("is_default = 1 AND deleted_at = 0").First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	return r.toDomain(&model), nil
}

func (r *UserRepository) List() ([]*domain.User, error) {
	var models []User
	if err := r.db.gorm.Where("deleted_at = 0").Order("id").Find(&models).Error; err != nil {
		return nil, err
	}
	users := make([]*domain.User, len(models))
	for i, m := range models {
		users[i] = r.toDomain(&m)
	}
	return users, nil
}

func (r *UserRepository) ListByTenant(tenantID uint64) ([]*domain.User, error) {
	var models []User
	if err := r.db.gorm.Where("tenant_id = ? AND deleted_at = 0", tenantID).Order("id").Find(&models).Error; err != nil {
		return nil, err
	}
	users := make([]*domain.User, len(models))
	for i, m := range models {
		users[i] = r.toDomain(&m)
	}
	return users, nil
}

func (r *UserRepository) toModel(u *domain.User) *User {
	isDefault := 0
	if u.IsDefault {
		isDefault = 1
	}
	status := string(u.Status)
	if status == "" {
		status = string(domain.UserStatusPending)
	}
	return &User{
		SoftDeleteModel: SoftDeleteModel{
			BaseModel: BaseModel{
				ID:        u.ID,
				CreatedAt: toTimestamp(u.CreatedAt),
				UpdatedAt: toTimestamp(u.UpdatedAt),
			},
			DeletedAt: toTimestampPtr(u.DeletedAt),
		},
		TenantID:           u.TenantID,
		Username:           u.Username,
		PasswordHash:       u.PasswordHash,
		PasskeyCredentials: LongText(u.PasskeyCredentials),
		Role:               string(u.Role),
		Status:             status,
		IsDefault:          isDefault,
		LastLoginAt:        toTimestampPtr(u.LastLoginAt),
	}
}

func (r *UserRepository) toDomain(m *User) *domain.User {
	status := domain.UserStatus(m.Status)
	if status != domain.UserStatusPending && status != domain.UserStatusActive {
		status = domain.UserStatusPending
	}
	return &domain.User{
		ID:                 m.ID,
		CreatedAt:          fromTimestamp(m.CreatedAt),
		UpdatedAt:          fromTimestamp(m.UpdatedAt),
		DeletedAt:          fromTimestampPtr(m.DeletedAt),
		TenantID:           m.TenantID,
		Username:           m.Username,
		PasswordHash:       m.PasswordHash,
		PasskeyCredentials: string(m.PasskeyCredentials),
		Role:               domain.UserRole(m.Role),
		Status:             status,
		IsDefault:          m.IsDefault == 1,
		LastLoginAt:        fromTimestampPtr(m.LastLoginAt),
	}
}

func (r *UserRepository) ListByTenantAndStatus(tenantID uint64, status domain.UserStatus) ([]*domain.User, error) {
	var models []User
	if err := r.db.gorm.Where("tenant_id = ? AND status = ? AND deleted_at = 0", tenantID, string(status)).Order("id").Find(&models).Error; err != nil {
		return nil, err
	}
	users := make([]*domain.User, len(models))
	for i, m := range models {
		users[i] = r.toDomain(&m)
	}
	return users, nil
}

func (r *UserRepository) CountActive() (int64, error) {
	var count int64
	if err := r.db.gorm.Model(&User{}).Where("status = ? AND deleted_at = 0", string(domain.UserStatusActive)).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}
