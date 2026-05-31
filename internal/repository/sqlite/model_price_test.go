package sqlite

import (
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
)

// TestUpdate_InsertsNewRow_PreservesHistory 验证版本化语义的核心契约:
// Update 不再原地覆盖,而是软删旧行 + 插入新行。旧行带 deleted_at != 0
// 留在表里(物理保留作为审计快照),GetByID 沿用"仅当前行"语义所以拿不
// 到——这与 admin Delete 后 404 的语义保持一致;历史反查若以后真有需要,
// 加独立方法,不污染 GetByID。
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

	// 旧行物理保留(deleted_at != 0)——直接查表验证审计快照仍在。
	var oldRow ModelPrice
	if err := db.gorm.Unscoped().First(&oldRow, originalID).Error; err != nil {
		t.Fatalf("raw query for old row id=%d: %v", originalID, err)
	}
	if oldRow.DeletedAt == 0 {
		t.Errorf("old row deleted_at = 0, want > 0 (should be soft-deleted)")
	}
	if oldRow.InputPriceMicro != 3_000_000 {
		t.Errorf("old row input price = %d, want 3_000_000 (历史值不应被覆盖)", oldRow.InputPriceMicro)
	}
	if oldRow.OutputPriceMicro != 15_000_000 {
		t.Errorf("old row output price = %d, want 15_000_000 (历史值不应被覆盖)", oldRow.OutputPriceMicro)
	}

	// GetByID(旧 ID)必须 not found——锁住"物理保留但不通过常规 admin/API
	// 读路径暴露"的契约,与 TestDeleteModelPrice 的 404 语义对齐。
	if got, err := repo.GetByID(originalID); err == nil {
		t.Errorf("GetByID(originalID=%d) after Update should fail (historical row must not leak through admin reads); got row %+v", originalID, got)
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

// TestGetByIDIncludingDeleted_ReturnsSoftDeletedRow 锁住"历史快照按 ID 反查"
// 的契约:Delete(软删) → GetByID 404,但 GetByIDIncludingDeleted 仍能取出
// 该行(且 deleted_at != 0)。RecalcAttemptUpdate / Calculator 的历史价反查
// 路径依赖这条性质;若有人无意中把 GetByIDIncludingDeleted 改成也过滤
// deleted_at,本测试会立刻报警。
func TestGetByIDIncludingDeleted_ReturnsSoftDeletedRow(t *testing.T) {
	db, err := NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	repo := NewModelPriceRepository(db)

	p := &domain.ModelPrice{
		ModelID:          "soft-delete-target",
		InputPriceMicro:  3_000_000,
		OutputPriceMicro: 15_000_000,
	}
	if err := repo.Create(p); err != nil {
		t.Fatalf("create: %v", err)
	}
	id := p.ID
	if id == 0 {
		t.Fatal("created ID should be set")
	}

	if err := repo.Delete(id); err != nil {
		t.Fatalf("delete: %v", err)
	}

	// GetByID 必须 404(admin 读路径不该看到软删行)
	if got, err := repo.GetByID(id); err == nil {
		t.Errorf("GetByID(id=%d) after Delete should fail; got %+v", id, got)
	}

	// GetByIDIncludingDeleted 必须返回完整行(历史快照仍可达)
	got, err := repo.GetByIDIncludingDeleted(id)
	if err != nil {
		t.Fatalf("GetByIDIncludingDeleted(id=%d) failed: %v", id, err)
	}
	if got == nil {
		t.Fatalf("GetByIDIncludingDeleted returned nil")
	}
	if got.ID != id {
		t.Errorf("returned ID = %d, want %d", got.ID, id)
	}
	if got.InputPriceMicro != 3_000_000 {
		t.Errorf("returned InputPriceMicro = %d, want 3_000_000 (历史价不应被改写)", got.InputPriceMicro)
	}
}
