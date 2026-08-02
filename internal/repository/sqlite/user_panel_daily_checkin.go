package sqlite

import (
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type UserPanelDailyCheckInRepository struct {
	db *DB
}

func NewUserPanelDailyCheckInRepository(db *DB) *UserPanelDailyCheckInRepository {
	return &UserPanelDailyCheckInRepository{db: db}
}

func (r *UserPanelDailyCheckInRepository) Claim(tenantID uint64, userID uint64, date string) (bool, error) {
	if tenantID == 0 || userID == 0 || date == "" {
		return false, nil
	}
	model := &UserPanelDailyCheckIn{
		TenantID: tenantID,
		UserID:   userID,
		Date:     date,
	}
	result := r.db.gorm.Clauses(clause.OnConflict{DoNothing: true}).Create(model)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrDuplicatedKey) {
			return false, nil
		}
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func (r *UserPanelDailyCheckInRepository) DeleteClaim(tenantID uint64, userID uint64, date string) error {
	if tenantID == 0 || userID == 0 || date == "" {
		return nil
	}
	return r.db.gorm.Where("tenant_id = ? AND user_id = ? AND date = ?", tenantID, userID, date).
		Delete(&UserPanelDailyCheckIn{}).Error
}
