package sqlite

import (
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository"
	"gorm.io/gorm"
)

type BedrockDiscoveryRepository struct {
	db *DB
}

func NewBedrockDiscoveryRepository(db *DB) repository.BedrockDiscoveryRepository {
	return &BedrockDiscoveryRepository{db: db}
}

func (r *BedrockDiscoveryRepository) Load(providerID uint64) ([]*domain.BedrockDiscoveryEntry, time.Time, error) {
	var rows []BedrockDiscoveryEntry
	if err := r.db.gorm.Where("provider_id = ?", providerID).Find(&rows).Error; err != nil {
		return nil, time.Time{}, err
	}
	out := make([]*domain.BedrockDiscoveryEntry, 0, len(rows))
	var newest time.Time
	for _, row := range rows {
		out = append(out, &domain.BedrockDiscoveryEntry{
			ShortName: row.ShortName,
			BedrockID: row.BedrockID,
			Source:    row.Source,
		})
		// All rows for one provider share a FetchedAt (Replace writes
		// them as a batch), but stay defensive — an old row left by a
		// partial write must not extend the TTL clock beyond the most
		// recent successful fetch.
		if ts := time.UnixMilli(row.FetchedAt); ts.After(newest) {
			newest = ts
		}
	}
	return out, newest, nil
}

func (r *BedrockDiscoveryRepository) Replace(providerID uint64, entries []*domain.BedrockDiscoveryEntry, fetchedAt time.Time) error {
	fetchedMs := fetchedAt.UnixMilli()
	return r.db.gorm.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("provider_id = ?", providerID).Delete(&BedrockDiscoveryEntry{}).Error; err != nil {
			return err
		}
		if len(entries) == 0 {
			return nil
		}
		rows := make([]BedrockDiscoveryEntry, 0, len(entries))
		for _, e := range entries {
			rows = append(rows, BedrockDiscoveryEntry{
				ProviderID: providerID,
				ShortName:  e.ShortName,
				BedrockID:  e.BedrockID,
				Source:     e.Source,
				FetchedAt:  fetchedMs,
			})
		}
		// No OnConflict clause — the preceding Delete already cleared
		// every (provider_id, short_name) row for this provider, so a
		// unique-index violation here would indicate a caller passed
		// duplicates in `entries` and should surface as an error rather
		// than be silently dropped.
		return tx.Create(&rows).Error
	})
}
