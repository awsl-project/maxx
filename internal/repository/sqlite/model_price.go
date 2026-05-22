package sqlite

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/pricing"
)

type ModelPriceRepository struct {
	db *DB
}

func NewModelPriceRepository(db *DB) *ModelPriceRepository {
	return &ModelPriceRepository{db: db}
}

// Create 创建新的价格记录
func (r *ModelPriceRepository) Create(price *domain.ModelPrice) error {
	m := r.fromDomain(price)
	if m.CreatedAt == 0 {
		m.CreatedAt = time.Now().UnixMilli()
	}
	if err := r.db.gorm.Create(m).Error; err != nil {
		return err
	}
	price.ID = m.ID
	price.CreatedAt = fromTimestamp(m.CreatedAt)
	return nil
}

// BatchCreate 批量创建价格记录
func (r *ModelPriceRepository) BatchCreate(prices []*domain.ModelPrice) error {
	if len(prices) == 0 {
		return nil
	}

	models := make([]*ModelPrice, len(prices))
	now := time.Now().UnixMilli()
	for i, p := range prices {
		m := r.fromDomain(p)
		if m.CreatedAt == 0 {
			m.CreatedAt = now
		}
		models[i] = m
	}

	if err := r.db.gorm.Create(&models).Error; err != nil {
		return err
	}

	// 更新原始对象的 ID 和 CreatedAt
	for i, m := range models {
		prices[i].ID = m.ID
		prices[i].CreatedAt = fromTimestamp(m.CreatedAt)
	}
	return nil
}

// GetByID 获取指定ID的价格记录(仅未软删的)。
//
// admin 单条编辑/查看路径,语义是"当前行";Delete 后再 GET 必须 404
// (e2e: TestDeleteModelPrice)。如果调用方需要拿到已被软删的历史快照行
// (例如按 attempt.ModelPriceID 反查当时的价格),用 GetByIDIncludingDeleted。
func (r *ModelPriceRepository) GetByID(id uint64) (*domain.ModelPrice, error) {
	var m ModelPrice
	if err := r.db.gorm.Where("deleted_at = 0").First(&m, id).Error; err != nil {
		return nil, err
	}
	return r.toDomain(&m), nil
}

// GetByIDIncludingDeleted 按 ID 取价格记录,包括已软删的历史版本。
//
// 专门给"按 attempt.ModelPriceID 反查当时价"用——版本化机制保证每次价
// 格变更都会软删旧行 + INSERT 新行,旧行物理保留作为审计快照。Calculator
// 通过 SetHistoricalLookup 注入此方法,实现按 ID 懒加载历史价格。
//
// 不暴露在 admin/API 读路径,避免被删的价格意外回流到 UI。
func (r *ModelPriceRepository) GetByIDIncludingDeleted(id uint64) (*domain.ModelPrice, error) {
	var m ModelPrice
	if err := r.db.gorm.First(&m, id).Error; err != nil {
		return nil, err
	}
	return r.toDomain(&m), nil
}

// GetCurrentByModelID 获取模型的当前价格（最新版本），支持前缀匹配。
//
// "当前版本" 统一以 id DESC 为序——id 是 autoincrement + unique，单调可靠；
// 不能用 created_at DESC，因为 BatchCreate（seed/reset）给整批同一个时间戳，
// 同 ms 内多次 Create 也会让 created_at 出现并列，排序结果不稳定。所有
// 涉及"取当前版本"的方法（Update、ListCurrentPrices、ListByModelID）都
// 对齐到 id 排序，避免不同读路径对同一 ModelID 拿到不同行。
func (r *ModelPriceRepository) GetCurrentByModelID(modelID string) (*domain.ModelPrice, error) {
	// 1. 精确匹配
	var exact ModelPrice
	err := r.db.gorm.Where("model_id = ? AND deleted_at = 0", modelID).
		Order("id DESC").
		First(&exact).Error
	if err == nil {
		return r.toDomain(&exact), nil
	}

	// 2. 前缀匹配：获取所有可能的前缀，找最长匹配
	var allPrices []ModelPrice
	if err := r.db.gorm.
		Where("deleted_at = 0").
		Select("DISTINCT model_id").
		Find(&allPrices).Error; err != nil {
		return nil, err
	}

	var bestMatch string
	for _, p := range allPrices {
		if strings.HasPrefix(modelID, p.ModelID) && len(p.ModelID) > len(bestMatch) {
			bestMatch = p.ModelID
		}
	}

	if bestMatch == "" {
		return nil, nil // 未找到匹配
	}

	// 获取最佳匹配的最新价格
	var m ModelPrice
	if err := r.db.gorm.Where("model_id = ? AND deleted_at = 0", bestMatch).
		Order("id DESC").
		First(&m).Error; err != nil {
		return nil, err
	}
	return r.toDomain(&m), nil
}

// ListCurrentPrices 获取所有模型的当前价格（每个 model_id 的最新记录）
func (r *ModelPriceRepository) ListCurrentPrices() ([]*domain.ModelPrice, error) {
	// 使用子查询获取每个 model_id 的最新 ID (只查询未删除的记录)
	subQuery := r.db.gorm.Model(&ModelPrice{}).
		Where("deleted_at = 0").
		Select("model_id, MAX(id) as max_id").
		Group("model_id")

	var models []ModelPrice
	if err := r.db.gorm.
		Joins("JOIN (?) AS latest ON model_prices.id = latest.max_id", subQuery).
		Where("model_prices.deleted_at = 0").
		Find(&models).Error; err != nil {
		return nil, err
	}

	result := make([]*domain.ModelPrice, len(models))
	for i, m := range models {
		result[i] = r.toDomain(&m)
	}
	return result, nil
}

// ListByModelID 获取模型的价格历史，最新版本优先（id DESC，与 GetCurrentByModelID 对齐）。
func (r *ModelPriceRepository) ListByModelID(modelID string) ([]*domain.ModelPrice, error) {
	var models []ModelPrice
	if err := r.db.gorm.Where("model_id = ? AND deleted_at = 0", modelID).
		Order("id DESC").
		Find(&models).Error; err != nil {
		return nil, err
	}

	result := make([]*domain.ModelPrice, len(models))
	for i, m := range models {
		result[i] = r.toDomain(&m)
	}
	return result, nil
}

// Count 获取价格记录总数
func (r *ModelPriceRepository) Count() (int64, error) {
	var count int64
	if err := r.db.gorm.Model(&ModelPrice{}).Where("deleted_at = 0").Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// Delete 软删除价格记录
func (r *ModelPriceRepository) Delete(id uint64) error {
	return r.db.gorm.Model(&ModelPrice{}).Where("id = ?", id).
		Update("deleted_at", time.Now().UnixMilli()).Error
}

// SoftDeleteAll 软删除所有价格记录
func (r *ModelPriceRepository) SoftDeleteAll() error {
	return r.db.gorm.Model(&ModelPrice{}).Where("deleted_at = 0").
		Update("deleted_at", time.Now().UnixMilli()).Error
}

// ResetToDefaults 重置为默认价格（软删除现有记录，插入默认价格）
func (r *ModelPriceRepository) ResetToDefaults() ([]*domain.ModelPrice, error) {
	// 1. 软删除所有现有记录
	if err := r.SoftDeleteAll(); err != nil {
		return nil, err
	}

	// 2. 从默认价格表获取价格并插入
	defaultTable := pricing.DefaultPriceTable()
	allPrices := defaultTable.All()

	domainPrices := make([]*domain.ModelPrice, 0, len(allPrices))
	for _, p := range allPrices {
		domainPrices = append(domainPrices, &domain.ModelPrice{
			ModelID:                p.ModelID,
			InputPriceMicro:        p.InputPriceMicro,
			OutputPriceMicro:       p.OutputPriceMicro,
			CacheReadPriceMicro:    p.CacheReadPriceMicro,
			Cache5mWritePriceMicro: p.Cache5mWritePriceMicro,
			Cache1hWritePriceMicro: p.Cache1hWritePriceMicro,
			Has1MContext:           p.Has1MContext,
			Context1MThreshold:     p.GetContext1MThreshold(),
			InputPremiumNum:        p.GetInputPremiumNum(),
			InputPremiumDenom:      p.GetInputPremiumDenom(),
			OutputPremiumNum:       p.GetOutputPremiumNum(),
			OutputPremiumDenom:     p.GetOutputPremiumDenom(),
		})
	}

	// 3. 批量插入
	if err := r.BatchCreate(domainPrices); err != nil {
		return nil, err
	}

	return domainPrices, nil
}

// Update 更新价格记录——版本化语义:软删同 ModelID 当前行,插入新行。
//
// 旧实现是 GORM Save 原地覆盖,导致历史 attempt 的 ModelPriceID 反查时拿到
// 被改后的新价格(成本审计/重算失真)。现在每次 Update:
//  1. 按 ModelID 找当前(未软删的最新)行
//  2. 与新价比较:若所有计费字段一致,直接 no-op(避免 UI 误点产生空版本)
//  3. 否则在同一事务里软删旧行 + INSERT 新行
//
// price.ID 在 no-op 情况下保留当前行 ID;在有变更时被改写为新插入的 ID,
// 调用方(admin handler)将这个新 ID 返回给前端。
func (r *ModelPriceRepository) Update(price *domain.ModelPrice) error {
	return r.db.gorm.Transaction(func(tx *gorm.DB) error {
		var current ModelPrice
		err := tx.Where("model_id = ? AND deleted_at = 0", price.ModelID).
			Order("id DESC").First(&current).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		if err == nil {
			if pricesEqual(&current, price) {
				price.ID = current.ID
				price.CreatedAt = fromTimestamp(current.CreatedAt)
				return nil
			}
			// 软删旧行
			now := time.Now().UnixMilli()
			if err := tx.Model(&ModelPrice{}).Where("id = ?", current.ID).
				Update("deleted_at", now).Error; err != nil {
				return err
			}
		}

		// 插新行
		m := r.fromDomain(price)
		m.ID = 0 // 强制生成新 ID
		m.DeletedAt = 0
		m.CreatedAt = time.Now().UnixMilli()
		if err := tx.Create(m).Error; err != nil {
			return err
		}
		price.ID = m.ID
		price.CreatedAt = fromTimestamp(m.CreatedAt)
		return nil
	})
}

// pricesEqual 比较 GORM 行的当前价与待写入 domain 价是否完全一致(所有计费/分层字段)。
// 仅当全相等时 Update 走 no-op 分支。
func pricesEqual(current *ModelPrice, next *domain.ModelPrice) bool {
	nextHas1M := 0
	if next.Has1MContext {
		nextHas1M = 1
	}
	return current.InputPriceMicro == next.InputPriceMicro &&
		current.OutputPriceMicro == next.OutputPriceMicro &&
		current.CacheReadPriceMicro == next.CacheReadPriceMicro &&
		current.Cache5mWritePriceMicro == next.Cache5mWritePriceMicro &&
		current.Cache1hWritePriceMicro == next.Cache1hWritePriceMicro &&
		current.Has1MContext == nextHas1M &&
		current.Context1MThreshold == next.Context1MThreshold &&
		current.InputPremiumNum == next.InputPremiumNum &&
		current.InputPremiumDenom == next.InputPremiumDenom &&
		current.OutputPremiumNum == next.OutputPremiumNum &&
		current.OutputPremiumDenom == next.OutputPremiumDenom
}

func (r *ModelPriceRepository) toDomain(m *ModelPrice) *domain.ModelPrice {
	return &domain.ModelPrice{
		ID:                     m.ID,
		CreatedAt:              fromTimestamp(m.CreatedAt),
		ModelID:                m.ModelID,
		InputPriceMicro:        m.InputPriceMicro,
		OutputPriceMicro:       m.OutputPriceMicro,
		CacheReadPriceMicro:    m.CacheReadPriceMicro,
		Cache5mWritePriceMicro: m.Cache5mWritePriceMicro,
		Cache1hWritePriceMicro: m.Cache1hWritePriceMicro,
		Has1MContext:           m.Has1MContext != 0,
		Context1MThreshold:     m.Context1MThreshold,
		InputPremiumNum:        m.InputPremiumNum,
		InputPremiumDenom:      m.InputPremiumDenom,
		OutputPremiumNum:       m.OutputPremiumNum,
		OutputPremiumDenom:     m.OutputPremiumDenom,
	}
}

func (r *ModelPriceRepository) fromDomain(p *domain.ModelPrice) *ModelPrice {
	has1MContext := 0
	if p.Has1MContext {
		has1MContext = 1
	}
	return &ModelPrice{
		ID:                     p.ID,
		CreatedAt:              toTimestamp(p.CreatedAt),
		ModelID:                p.ModelID,
		InputPriceMicro:        p.InputPriceMicro,
		OutputPriceMicro:       p.OutputPriceMicro,
		CacheReadPriceMicro:    p.CacheReadPriceMicro,
		Cache5mWritePriceMicro: p.Cache5mWritePriceMicro,
		Cache1hWritePriceMicro: p.Cache1hWritePriceMicro,
		Has1MContext:           has1MContext,
		Context1MThreshold:     p.Context1MThreshold,
		InputPremiumNum:        p.InputPremiumNum,
		InputPremiumDenom:      p.InputPremiumDenom,
		OutputPremiumNum:       p.OutputPremiumNum,
		OutputPremiumDenom:     p.OutputPremiumDenom,
	}
}
