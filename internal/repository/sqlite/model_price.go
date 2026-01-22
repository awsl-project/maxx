package sqlite

import (
	"strings"
	"time"

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

// GetByID 获取指定ID的价格记录
func (r *ModelPriceRepository) GetByID(id uint64) (*domain.ModelPrice, error) {
	var m ModelPrice
	if err := r.db.gorm.Where("deleted_at = 0").First(&m, id).Error; err != nil {
		return nil, err
	}
	return r.toDomain(&m), nil
}

// GetCurrentByModelID 获取模型的当前价格（最新记录），支持前缀匹配
func (r *ModelPriceRepository) GetCurrentByModelID(modelID string) (*domain.ModelPrice, error) {
	// 1. 精确匹配
	var exact ModelPrice
	err := r.db.gorm.Where("model_id = ? AND deleted_at = 0", modelID).
		Order("created_at DESC").
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
		Order("created_at DESC").
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

// ListByModelID 获取模型的价格历史
func (r *ModelPriceRepository) ListByModelID(modelID string) ([]*domain.ModelPrice, error) {
	var models []ModelPrice
	if err := r.db.gorm.Where("model_id = ? AND deleted_at = 0", modelID).
		Order("created_at DESC").
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

// Update 更新价格记录
func (r *ModelPriceRepository) Update(price *domain.ModelPrice) error {
	m := r.fromDomain(price)
	return r.db.gorm.Save(m).Error
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
