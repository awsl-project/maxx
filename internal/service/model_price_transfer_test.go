package service

import (
	"encoding/json"
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository/sqlite"
)

func newAdminServiceForModelPriceTransferTest(t *testing.T) (*AdminService, *sqlite.ModelPriceRepository) {
	t.Helper()

	db, err := sqlite.NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("NewDBWithDSN() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	repo := sqlite.NewModelPriceRepository(db)
	svc := NewAdminService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, repo, nil, "", nil, nil, nil)
	return svc, repo
}

func uint64Ptr(v uint64) *uint64 { return &v }
func boolPtr(v bool) *bool       { return &v }

func TestAdminServiceExportModelPricesShapeOmitsDatabaseFields(t *testing.T) {
	svc, repo := newAdminServiceForModelPriceTransferTest(t)

	if err := repo.Create(&domain.ModelPrice{
		ModelID:                "gpt-image-2",
		InputPriceMicro:        2_000_000,
		OutputPriceMicro:       8_000_000,
		CacheReadPriceMicro:    200_000,
		Cache5mWritePriceMicro: 2_500_000,
		Cache1hWritePriceMicro: 4_000_000,
		ImageInputPriceMicro:   10_000_000,
		ImageOutputPriceMicro:  40_000_000,
		Has1MContext:           true,
		Context1MThreshold:     200_000,
		InputPremiumNum:        2,
		InputPremiumDenom:      1,
		OutputPremiumNum:       3,
		OutputPremiumDenom:     2,
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	file, err := svc.ExportModelPrices()
	if err != nil {
		t.Fatalf("ExportModelPrices() error = %v", err)
	}
	if file.Type != ModelPriceExportType || file.Version != ModelPriceExportVersion {
		t.Fatalf("unexpected envelope: %+v", file)
	}
	if len(file.Prices) != 1 {
		t.Fatalf("prices len = %d, want 1", len(file.Prices))
	}
	if file.Prices[0].ImageInputPriceMicro != 10_000_000 || file.Prices[0].ImageOutputPriceMicro != 40_000_000 {
		t.Fatalf("image prices not exported: %+v", file.Prices[0])
	}

	payload, err := json.Marshal(file.Prices[0])
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(payload, &raw); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	for _, forbidden := range []string{"id", "createdAt", "updatedAt", "deletedAt"} {
		if _, ok := raw[forbidden]; ok {
			t.Fatalf("exported database field %q in %v", forbidden, raw)
		}
	}
}

func TestAdminServiceImportModelPricesValidationAndDryRun(t *testing.T) {
	svc, repo := newAdminServiceForModelPriceTransferTest(t)

	file := ModelPriceImportFile{
		Type:    ModelPriceExportType,
		Version: ModelPriceExportVersion,
		Prices: []ModelPriceImportEntry{{
			ModelID:            "claude-sonnet-4",
			InputPriceMicro:    uint64Ptr(3_000_000),
			OutputPriceMicro:   uint64Ptr(15_000_000),
			InputPremiumDenom:  uint64Ptr(1),
			OutputPremiumDenom: uint64Ptr(1),
		}, {
			ModelID:          "claude-sonnet-4",
			InputPriceMicro:  uint64Ptr(4_000_000),
			OutputPriceMicro: uint64Ptr(16_000_000),
		}},
	}

	result, err := svc.ImportModelPrices(file, ModelPriceImportOptions{DryRun: true})
	if err != nil {
		t.Fatalf("ImportModelPrices() error = %v", err)
	}
	if result.Success || len(result.Errors) == 0 {
		t.Fatalf("result = %+v, want duplicate validation error", result)
	}

	file.Prices = file.Prices[:1]
	result, err = svc.ImportModelPrices(file, ModelPriceImportOptions{DryRun: true})
	if err != nil {
		t.Fatalf("ImportModelPrices() dry-run error = %v", err)
	}
	if !result.Success || result.Summary.Imported != 1 {
		t.Fatalf("dry-run result = %+v, want one imported preview", result)
	}
	count, err := repo.Count()
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("repo count = %d, want dry-run to write nothing", count)
	}
}

func TestAdminServiceImportModelPricesRejectsOverflowingDefaultCachePrices(t *testing.T) {
	svc, repo := newAdminServiceForModelPriceTransferTest(t)

	file := ModelPriceImportFile{
		Type:    ModelPriceExportType,
		Version: ModelPriceExportVersion,
		Prices: []ModelPriceImportEntry{{
			ModelID:          "huge-input",
			InputPriceMicro:  uint64Ptr(^uint64(0)),
			OutputPriceMicro: uint64Ptr(1),
		}},
	}

	result, err := svc.ImportModelPrices(file, ModelPriceImportOptions{})
	if err != nil {
		t.Fatalf("ImportModelPrices() error = %v", err)
	}
	if result.Success || len(result.Errors) == 0 {
		t.Fatalf("result = %+v, want overflow validation error", result)
	}
	count, err := repo.Count()
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("repo count = %d, want no writes", count)
	}
}

func TestAdminServiceImportModelPricesErrorStrategyPreflightsConflictsBeforeWrites(t *testing.T) {
	svc, repo := newAdminServiceForModelPriceTransferTest(t)

	if err := repo.Create(&domain.ModelPrice{
		ModelID:                "existing-model",
		InputPriceMicro:        1,
		OutputPriceMicro:       2,
		CacheReadPriceMicro:    0,
		Cache5mWritePriceMicro: 1,
		Cache1hWritePriceMicro: 2,
		Context1MThreshold:     200_000,
		InputPremiumNum:        2,
		InputPremiumDenom:      1,
		OutputPremiumNum:       2,
		OutputPremiumDenom:     1,
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	file := ModelPriceImportFile{
		Type:    ModelPriceExportType,
		Version: ModelPriceExportVersion,
		Prices: []ModelPriceImportEntry{{
			ModelID:          "new-model",
			InputPriceMicro:  uint64Ptr(3),
			OutputPriceMicro: uint64Ptr(4),
		}, {
			ModelID:          "existing-model",
			InputPriceMicro:  uint64Ptr(5),
			OutputPriceMicro: uint64Ptr(6),
		}},
	}

	result, err := svc.ImportModelPrices(file, ModelPriceImportOptions{ConflictStrategy: "error"})
	if err != nil {
		t.Fatalf("ImportModelPrices() error = %v", err)
	}
	if result.Success || len(result.Errors) == 0 {
		t.Fatalf("result = %+v, want conflict error", result)
	}
	count, err := repo.Count()
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("repo count = %d, want preflight conflict to prevent partial writes", count)
	}
	created, err := repo.GetCurrentByModelID("new-model")
	if err != nil {
		t.Fatalf("GetCurrentByModelID(new-model) error = %v", err)
	}
	if created != nil {
		t.Fatalf("new-model was partially written: %+v", created)
	}
}

func TestAdminServiceImportModelPricesConflictStrategies(t *testing.T) {
	svc, repo := newAdminServiceForModelPriceTransferTest(t)

	existing := &domain.ModelPrice{
		ModelID:                "gpt-4o",
		InputPriceMicro:        1_000_000,
		OutputPriceMicro:       2_000_000,
		CacheReadPriceMicro:    100_000,
		Cache5mWritePriceMicro: 1_250_000,
		Cache1hWritePriceMicro: 2_000_000,
		Context1MThreshold:     200_000,
		InputPremiumNum:        2,
		InputPremiumDenom:      1,
		OutputPremiumNum:       2,
		OutputPremiumDenom:     1,
	}
	if err := repo.Create(existing); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	oldID := existing.ID

	file := ModelPriceImportFile{
		Type:    ModelPriceExportType,
		Version: ModelPriceExportVersion,
		Prices: []ModelPriceImportEntry{{
			ModelID:                "gpt-4o",
			InputPriceMicro:        uint64Ptr(3_000_000),
			OutputPriceMicro:       uint64Ptr(9_000_000),
			CacheReadPriceMicro:    uint64Ptr(300_000),
			Cache5mWritePriceMicro: uint64Ptr(3_750_000),
			Cache1hWritePriceMicro: uint64Ptr(6_000_000),
			Has1MContext:           boolPtr(false),
			Context1MThreshold:     uint64Ptr(200_000),
			InputPremiumNum:        uint64Ptr(2),
			InputPremiumDenom:      uint64Ptr(1),
			OutputPremiumNum:       uint64Ptr(2),
			OutputPremiumDenom:     uint64Ptr(1),
		}},
	}

	result, err := svc.ImportModelPrices(file, ModelPriceImportOptions{ConflictStrategy: "skip"})
	if err != nil {
		t.Fatalf("ImportModelPrices(skip) error = %v", err)
	}
	if !result.Success || result.Summary.Skipped != 1 {
		t.Fatalf("skip result = %+v, want one skipped", result)
	}
	current, err := repo.GetCurrentByModelID("gpt-4o")
	if err != nil {
		t.Fatalf("GetCurrentByModelID() error = %v", err)
	}
	if current.InputPriceMicro != 1_000_000 || current.ID != oldID {
		t.Fatalf("current after skip = %+v, want original", current)
	}

	result, err = svc.ImportModelPrices(file, ModelPriceImportOptions{ConflictStrategy: "error"})
	if err != nil {
		t.Fatalf("ImportModelPrices(error) error = %v", err)
	}
	if result.Success || len(result.Errors) == 0 {
		t.Fatalf("error strategy result = %+v, want conflict error", result)
	}

	result, err = svc.ImportModelPrices(file, ModelPriceImportOptions{ConflictStrategy: "overwrite"})
	if err != nil {
		t.Fatalf("ImportModelPrices(overwrite) error = %v", err)
	}
	if !result.Success || result.Summary.Updated != 1 {
		t.Fatalf("overwrite result = %+v, want one updated", result)
	}
	current, err = repo.GetCurrentByModelID("gpt-4o")
	if err != nil {
		t.Fatalf("GetCurrentByModelID() after overwrite error = %v", err)
	}
	if current.InputPriceMicro != 3_000_000 {
		t.Fatalf("current input price = %d, want overwritten value", current.InputPriceMicro)
	}
	if current.ID == oldID {
		t.Fatalf("current ID = old ID %d, want versioned replacement", oldID)
	}
}
