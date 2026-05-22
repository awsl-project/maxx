package sqlite

import (
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
)

// TestUpdate_InsertsNewRow_PreservesHistory 验证版本化语义的核心契约:
// Update 不再原地覆盖,而是软删旧行 + 插入新行;旧 ID 仍可通过 GetByID
// 取出当时的价格快照,历史 attempt 反查不会失真。
func TestUpdate_InsertsNewRow_PreservesHistory(t *testing.T) {
	db, err := NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	repo := NewModelPriceRepository(db)

	original := &domain.ModelPrice{
		ModelID:                "test-model",
		InputPriceMicro:        3_000_000,
		OutputPriceMicro:       15_000_000,
		CacheReadPriceMicro:    300_000,
		Cache5mWritePriceMicro: 3_750_000,
		Cache1hWritePriceMicro: 3_750_000,
	}
	if err := repo.Create(original); err != nil {
		t.Fatalf("create original: %v", err)
	}
	originalID := original.ID
	if originalID == 0 {
		t.Fatal("original ID should be set after Create")
	}

	updated := &domain.ModelPrice{
		ModelID:                "test-model",
		InputPriceMicro:        4_000_000, // changed
		OutputPriceMicro:       16_000_000,
		CacheReadPriceMicro:    400_000,
		Cache5mWritePriceMicro: 4_000_000,
		Cache1hWritePriceMicro: 4_000_000,
	}
	if err := repo.Update(updated); err != nil {
		t.Fatalf("update: %v", err)
	}

	if updated.ID == originalID {
		t.Fatalf("updated ID should differ from original; got both = %d", originalID)
	}
	if updated.ID == 0 {
		t.Fatal("updated ID should be set")
	}

	// 旧 ID 仍能反查到旧价格(历史快照)
	old, err := repo.GetByID(originalID)
	if err != nil {
		t.Fatalf("GetByID(originalID) after update should succeed (historical snapshot); got: %v", err)
	}
	if old.InputPriceMicro != 3_000_000 {
		t.Errorf("historical input price = %d, want 3_000_000 (unchanged)", old.InputPriceMicro)
	}
	if old.OutputPriceMicro != 15_000_000 {
		t.Errorf("historical output price = %d, want 15_000_000 (unchanged)", old.OutputPriceMicro)
	}

	// GetCurrentByModelID 返回新价格
	current, err := repo.GetCurrentByModelID("test-model")
	if err != nil {
		t.Fatalf("GetCurrentByModelID: %v", err)
	}
	if current.ID != updated.ID {
		t.Errorf("current ID = %d, want %d (just-inserted new row)", current.ID, updated.ID)
	}
	if current.InputPriceMicro != 4_000_000 {
		t.Errorf("current input price = %d, want 4_000_000 (new)", current.InputPriceMicro)
	}

	// ListCurrentPrices 只返回当前行,不返回历史
	currents, err := repo.ListCurrentPrices()
	if err != nil {
		t.Fatalf("ListCurrentPrices: %v", err)
	}
	if len(currents) != 1 {
		t.Fatalf("ListCurrentPrices returned %d rows, want 1", len(currents))
	}
	if currents[0].ID != updated.ID {
		t.Errorf("ListCurrentPrices ID = %d, want %d", currents[0].ID, updated.ID)
	}
}

// TestUpdate_NoOpWhenUnchanged 验证所有字段未变时 Update 不产生新行——避免
// UI 反复保存或回放定时同步任务时无意义版本爆炸。
func TestUpdate_NoOpWhenUnchanged(t *testing.T) {
	db, err := NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	repo := NewModelPriceRepository(db)

	p := &domain.ModelPrice{
		ModelID:                "test-model",
		InputPriceMicro:        3_000_000,
		OutputPriceMicro:       15_000_000,
		CacheReadPriceMicro:    300_000,
		Cache5mWritePriceMicro: 3_750_000,
		Cache1hWritePriceMicro: 3_750_000,
		Has1MContext:           true,
		Context1MThreshold:     200_000,
		InputPremiumNum:        2,
		InputPremiumDenom:      1,
		OutputPremiumNum:       15,
		OutputPremiumDenom:     10,
	}
	if err := repo.Create(p); err != nil {
		t.Fatalf("create: %v", err)
	}
	originalID := p.ID

	// 用全等的字段再次 Update
	echo := &domain.ModelPrice{
		ModelID:                "test-model",
		InputPriceMicro:        3_000_000,
		OutputPriceMicro:       15_000_000,
		CacheReadPriceMicro:    300_000,
		Cache5mWritePriceMicro: 3_750_000,
		Cache1hWritePriceMicro: 3_750_000,
		Has1MContext:           true,
		Context1MThreshold:     200_000,
		InputPremiumNum:        2,
		InputPremiumDenom:      1,
		OutputPremiumNum:       15,
		OutputPremiumDenom:     10,
	}
	if err := repo.Update(echo); err != nil {
		t.Fatalf("update (no-op): %v", err)
	}

	if echo.ID != originalID {
		t.Errorf("no-op update should keep ID = %d; got %d", originalID, echo.ID)
	}

	// 仍然只有一行
	count, err := repo.Count()
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("after no-op Update, count = %d, want 1", count)
	}

	// 再改一个字段——应该产生新行
	echo.InputPriceMicro = 3_500_000
	if err := repo.Update(echo); err != nil {
		t.Fatalf("update (real change): %v", err)
	}
	if echo.ID == originalID {
		t.Errorf("real-change Update should produce new ID; got original %d", originalID)
	}

	count, err = repo.Count()
	if err != nil {
		t.Fatalf("count after real change: %v", err)
	}
	if count != 1 {
		t.Errorf("after real-change Update, current row count = %d, want 1 (旧行已软删,不计入)", count)
	}
}
