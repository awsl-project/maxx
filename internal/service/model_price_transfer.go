package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

const ModelPriceExportType = "maxx.model-prices"
const ModelPriceExportVersion = 1

type ModelPriceExportFile struct {
	Type       string                  `json:"type"`
	Version    int                     `json:"version"`
	ExportedAt time.Time               `json:"exportedAt"`
	Prices     []ModelPriceExportEntry `json:"prices"`
}

type ModelPriceExportEntry struct {
	ModelID                string `json:"modelId"`
	InputPriceMicro        uint64 `json:"inputPriceMicro"`
	OutputPriceMicro       uint64 `json:"outputPriceMicro"`
	CacheReadPriceMicro    uint64 `json:"cacheReadPriceMicro"`
	Cache5mWritePriceMicro uint64 `json:"cache5mWritePriceMicro"`
	Cache1hWritePriceMicro uint64 `json:"cache1hWritePriceMicro"`
	ImageInputPriceMicro   uint64 `json:"imageInputPriceMicro"`
	ImageOutputPriceMicro  uint64 `json:"imageOutputPriceMicro"`
	Has1MContext           bool   `json:"has1mContext"`
	Context1MThreshold     uint64 `json:"context1mThreshold"`
	InputPremiumNum        uint64 `json:"inputPremiumNum"`
	InputPremiumDenom      uint64 `json:"inputPremiumDenom"`
	OutputPremiumNum       uint64 `json:"outputPremiumNum"`
	OutputPremiumDenom     uint64 `json:"outputPremiumDenom"`
}

type ModelPriceImportFile struct {
	Type       string                  `json:"type"`
	Version    int                     `json:"version"`
	ExportedAt time.Time               `json:"exportedAt,omitempty"`
	Prices     []ModelPriceImportEntry `json:"prices"`
}

type ModelPriceImportEntry struct {
	ModelID                string  `json:"modelId"`
	InputPriceMicro        *uint64 `json:"inputPriceMicro"`
	OutputPriceMicro       *uint64 `json:"outputPriceMicro"`
	CacheReadPriceMicro    *uint64 `json:"cacheReadPriceMicro"`
	Cache5mWritePriceMicro *uint64 `json:"cache5mWritePriceMicro"`
	Cache1hWritePriceMicro *uint64 `json:"cache1hWritePriceMicro"`
	ImageInputPriceMicro   *uint64 `json:"imageInputPriceMicro"`
	ImageOutputPriceMicro  *uint64 `json:"imageOutputPriceMicro"`
	Has1MContext           *bool   `json:"has1mContext"`
	Context1MThreshold     *uint64 `json:"context1mThreshold"`
	InputPremiumNum        *uint64 `json:"inputPremiumNum"`
	InputPremiumDenom      *uint64 `json:"inputPremiumDenom"`
	OutputPremiumNum       *uint64 `json:"outputPremiumNum"`
	OutputPremiumDenom     *uint64 `json:"outputPremiumDenom"`
}

type ModelPriceImportOptions struct {
	ConflictStrategy string
	DryRun           bool
}

type ModelPriceImportSummary struct {
	Imported int `json:"imported"`
	Skipped  int `json:"skipped"`
	Updated  int `json:"updated"`
}

type ModelPriceImportResult struct {
	Success  bool                    `json:"success"`
	Summary  ModelPriceImportSummary `json:"summary"`
	Errors   []string                `json:"errors"`
	Warnings []string                `json:"warnings"`
}

func (s *AdminService) ExportModelPrices() (*ModelPriceExportFile, error) {
	prices, err := s.modelPriceRepo.ListCurrentPrices()
	if err != nil {
		return nil, err
	}

	entries := make([]ModelPriceExportEntry, 0, len(prices))
	for _, price := range prices {
		entries = append(entries, ModelPriceExportEntry{
			ModelID:                price.ModelID,
			InputPriceMicro:        price.InputPriceMicro,
			OutputPriceMicro:       price.OutputPriceMicro,
			CacheReadPriceMicro:    price.CacheReadPriceMicro,
			Cache5mWritePriceMicro: price.Cache5mWritePriceMicro,
			Cache1hWritePriceMicro: price.Cache1hWritePriceMicro,
			ImageInputPriceMicro:   price.ImageInputPriceMicro,
			ImageOutputPriceMicro:  price.ImageOutputPriceMicro,
			Has1MContext:           price.Has1MContext,
			Context1MThreshold:     price.Context1MThreshold,
			InputPremiumNum:        price.InputPremiumNum,
			InputPremiumDenom:      price.InputPremiumDenom,
			OutputPremiumNum:       price.OutputPremiumNum,
			OutputPremiumDenom:     price.OutputPremiumDenom,
		})
	}

	return &ModelPriceExportFile{
		Type:       ModelPriceExportType,
		Version:    ModelPriceExportVersion,
		ExportedAt: time.Now().UTC(),
		Prices:     entries,
	}, nil
}

func (s *AdminService) ImportModelPrices(file ModelPriceImportFile, opts ModelPriceImportOptions) (*ModelPriceImportResult, error) {
	result := &ModelPriceImportResult{Success: true, Errors: []string{}, Warnings: []string{}}
	strategy := strings.TrimSpace(opts.ConflictStrategy)
	if strategy == "" {
		strategy = "skip"
	}
	if strategy != "skip" && strategy != "overwrite" && strategy != "error" {
		result.Success = false
		result.Errors = append(result.Errors, "invalid conflictStrategy: expected skip, overwrite, or error")
		return result, nil
	}
	if file.Type != ModelPriceExportType {
		result.Success = false
		result.Errors = append(result.Errors, fmt.Sprintf("invalid type: expected %s", ModelPriceExportType))
		return result, nil
	}
	if file.Version != ModelPriceExportVersion {
		result.Success = false
		result.Errors = append(result.Errors, fmt.Sprintf("unsupported version: %d", file.Version))
		return result, nil
	}

	prices, warnings, errors := normalizeImportedModelPrices(file.Prices)
	result.Warnings = append(result.Warnings, warnings...)
	if len(errors) > 0 {
		result.Success = false
		result.Errors = append(result.Errors, errors...)
		return result, nil
	}

	existingPrices, err := s.modelPriceRepo.ListCurrentPrices()
	if err != nil {
		return nil, err
	}
	existingByModelID := make(map[string]*domain.ModelPrice, len(existingPrices))
	for _, existing := range existingPrices {
		existingByModelID[existing.ModelID] = existing
	}

	if strategy == "error" {
		for _, price := range prices {
			if _, exists := existingByModelID[price.ModelID]; exists {
				result.Success = false
				result.Errors = append(result.Errors, fmt.Sprintf("model price conflict: %s already exists", price.ModelID))
			}
		}
		if len(result.Errors) > 0 {
			return result, nil
		}
	}

	for _, price := range prices {
		existing, exists := existingByModelID[price.ModelID]
		if exists {
			switch strategy {
			case "skip":
				result.Summary.Skipped++
				continue
			case "error":
				// Conflicts were preflighted before writes, so this can only be hit
				// by a duplicate introduced by this same import payload.
				result.Success = false
				result.Errors = append(result.Errors, fmt.Sprintf("model price conflict: %s already exists", price.ModelID))
				continue
			case "overwrite":
				price.ID = existing.ID
				price.CreatedAt = existing.CreatedAt
				if !opts.DryRun {
					if err := s.modelPriceRepo.Update(price); err != nil {
						result.Success = false
						result.Errors = append(result.Errors, fmt.Sprintf("failed to update model price %s: %v", price.ModelID, err))
						continue
					}
					existingByModelID[price.ModelID] = price
				}
				result.Summary.Updated++
				continue
			}
		}

		if !opts.DryRun {
			if err := s.modelPriceRepo.Create(price); err != nil {
				result.Success = false
				result.Errors = append(result.Errors, fmt.Sprintf("failed to import model price %s: %v", price.ModelID, err))
				continue
			}
			existingByModelID[price.ModelID] = price
		}
		result.Summary.Imported++
	}

	return result, nil
}

func normalizeImportedModelPrices(entries []ModelPriceImportEntry) ([]*domain.ModelPrice, []string, []string) {
	seen := make(map[string]struct{}, len(entries))
	prices := make([]*domain.ModelPrice, 0, len(entries))
	warnings := []string{}
	errors := []string{}

	for i, entry := range entries {
		line := fmt.Sprintf("prices[%d]", i)
		modelID := strings.TrimSpace(entry.ModelID)
		if modelID == "" {
			errors = append(errors, line+": modelId is required")
			continue
		}
		if _, ok := seen[modelID]; ok {
			errors = append(errors, fmt.Sprintf("%s: duplicate modelId %q", line, modelID))
			continue
		}
		seen[modelID] = struct{}{}

		if entry.InputPriceMicro == nil {
			errors = append(errors, line+": inputPriceMicro is required")
		}
		if entry.OutputPriceMicro == nil {
			errors = append(errors, line+": outputPriceMicro is required")
		}
		if entry.InputPriceMicro == nil || entry.OutputPriceMicro == nil {
			continue
		}

		input := *entry.InputPriceMicro
		output := *entry.OutputPriceMicro
		cacheRead := valueOrDefault(entry.CacheReadPriceMicro, input/10, line, "cacheReadPriceMicro", &warnings)
		cache5mFallback, ok := defaultCache5mWritePrice(input)
		if !ok && entry.Cache5mWritePriceMicro == nil {
			errors = append(errors, line+": inputPriceMicro is too large to default cache5mWritePriceMicro")
			continue
		}
		cache5m := valueOrDefault(entry.Cache5mWritePriceMicro, cache5mFallback, line, "cache5mWritePriceMicro", &warnings)
		cache1hFallback, ok := defaultCache1hWritePrice(input)
		if !ok && entry.Cache1hWritePriceMicro == nil {
			errors = append(errors, line+": inputPriceMicro is too large to default cache1hWritePriceMicro")
			continue
		}
		cache1h := valueOrDefault(entry.Cache1hWritePriceMicro, cache1hFallback, line, "cache1hWritePriceMicro", &warnings)
		imageInput := valueOrDefault(entry.ImageInputPriceMicro, 0, line, "imageInputPriceMicro", &warnings)
		imageOutput := valueOrDefault(entry.ImageOutputPriceMicro, 0, line, "imageOutputPriceMicro", &warnings)
		has1m := boolValueOrDefault(entry.Has1MContext, false, line, "has1mContext", &warnings)
		threshold := valueOrDefault(entry.Context1MThreshold, 200000, line, "context1mThreshold", &warnings)
		inputNum := valueOrDefault(entry.InputPremiumNum, 2, line, "inputPremiumNum", &warnings)
		inputDenom := valueOrDefault(entry.InputPremiumDenom, 1, line, "inputPremiumDenom", &warnings)
		outputNum := valueOrDefault(entry.OutputPremiumNum, 2, line, "outputPremiumNum", &warnings)
		outputDenom := valueOrDefault(entry.OutputPremiumDenom, 1, line, "outputPremiumDenom", &warnings)

		if inputDenom == 0 {
			errors = append(errors, line+": inputPremiumDenom must be greater than zero")
		}
		if outputDenom == 0 {
			errors = append(errors, line+": outputPremiumDenom must be greater than zero")
		}
		if inputDenom == 0 || outputDenom == 0 {
			continue
		}

		prices = append(prices, &domain.ModelPrice{
			ModelID:                modelID,
			InputPriceMicro:        input,
			OutputPriceMicro:       output,
			CacheReadPriceMicro:    cacheRead,
			Cache5mWritePriceMicro: cache5m,
			Cache1hWritePriceMicro: cache1h,
			ImageInputPriceMicro:   imageInput,
			ImageOutputPriceMicro:  imageOutput,
			Has1MContext:           has1m,
			Context1MThreshold:     threshold,
			InputPremiumNum:        inputNum,
			InputPremiumDenom:      inputDenom,
			OutputPremiumNum:       outputNum,
			OutputPremiumDenom:     outputDenom,
		})
	}

	return prices, warnings, errors
}

func defaultCache5mWritePrice(input uint64) (uint64, bool) {
	max := ^uint64(0)
	if input > (max/5)*4 {
		return 0, false
	}
	return (input / 4 * 5) + (input%4)*5/4, true
}

func defaultCache1hWritePrice(input uint64) (uint64, bool) {
	max := ^uint64(0)
	if input > max/2 {
		return 0, false
	}
	return input * 2, true
}

func valueOrDefault(value *uint64, fallback uint64, location string, field string, warnings *[]string) uint64 {
	if value != nil {
		return *value
	}
	*warnings = append(*warnings, fmt.Sprintf("%s: defaulted %s to %d", location, field, fallback))
	return fallback
}

func boolValueOrDefault(value *bool, fallback bool, location string, field string, warnings *[]string) bool {
	if value != nil {
		return *value
	}
	*warnings = append(*warnings, fmt.Sprintf("%s: defaulted %s to %t", location, field, fallback))
	return fallback
}
