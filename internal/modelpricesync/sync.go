package modelpricesync

import (
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository"
)

// Result summarizes previewing or applying prices into the DB.
type Result struct {
	Source    string               `json:"source"`
	SourceURL string               `json:"sourceUrl"`
	Total     int                  `json:"total"`
	Created   int                  `json:"created"`
	Updated   int                  `json:"updated"`
	Skipped   int                  `json:"skipped"`
	Changes   []Change             `json:"changes"`
	Prices    []*domain.ModelPrice `json:"prices,omitempty"`
}

// Change describes one pending or applied model price change.
type Change struct {
	Action string             `json:"action"`
	Before *domain.ModelPrice `json:"before,omitempty"`
	After  *domain.ModelPrice `json:"after"`
}

// Preview fetches source prices and returns pending DB changes without applying them.
func Preview(repo repository.ModelPriceRepository, sourceCode string) (*Result, error) {
	return sync(repo, sourceCode, false)
}

// Apply fetches source prices and applies missing or changed prices.
// Custom DB-only prices are preserved.
func Apply(repo repository.ModelPriceRepository, sourceCode string) (*Result, error) {
	return sync(repo, sourceCode, true)
}

func sync(repo repository.ModelPriceRepository, sourceCode string, apply bool) (*Result, error) {
	sourcePrices, source, sourceURL, err := Fetch(sourceCode)
	if err != nil {
		return nil, err
	}

	result := &Result{Source: source.Code, SourceURL: sourceURL, Total: len(sourcePrices)}
	currentPrices, err := repo.ListCurrentPrices()
	if err != nil {
		return nil, err
	}
	currentByModelID := make(map[string]*domain.ModelPrice, len(currentPrices))
	for _, price := range currentPrices {
		currentByModelID[price.ModelID] = price
	}

	for _, price := range sourcePrices {
		current := currentByModelID[price.ModelID]

		if current == nil {
			if apply {
				if err := repo.Create(price); err != nil {
					return nil, err
				}
				currentByModelID[price.ModelID] = price
			}
			result.Created++
			result.Changes = append(result.Changes, Change{Action: "create", After: cloneModelPrice(price)})
			continue
		}

		if modelPricesEqual(current, price) {
			result.Skipped++
			continue
		}

		price.ID = current.ID
		if apply {
			if err := repo.Update(price); err != nil {
				return nil, err
			}
			currentByModelID[price.ModelID] = price
		}
		result.Updated++
		result.Changes = append(result.Changes, Change{Action: "update", Before: cloneModelPrice(current), After: cloneModelPrice(price)})
	}

	if apply {
		prices, err := repo.ListCurrentPrices()
		if err != nil {
			return nil, err
		}
		result.Prices = prices
	}
	return result, nil
}

func cloneModelPrice(price *domain.ModelPrice) *domain.ModelPrice {
	if price == nil {
		return nil
	}
	clone := *price
	return &clone
}

func modelPricesEqual(a, b *domain.ModelPrice) bool {
	return a.InputPriceMicro == b.InputPriceMicro &&
		a.OutputPriceMicro == b.OutputPriceMicro &&
		a.CacheReadPriceMicro == b.CacheReadPriceMicro &&
		a.Cache5mWritePriceMicro == b.Cache5mWritePriceMicro &&
		a.Cache1hWritePriceMicro == b.Cache1hWritePriceMicro &&
		a.ImageInputPriceMicro == b.ImageInputPriceMicro &&
		a.ImageOutputPriceMicro == b.ImageOutputPriceMicro &&
		a.Has1MContext == b.Has1MContext &&
		a.Context1MThreshold == b.Context1MThreshold &&
		a.InputPremiumNum == b.InputPremiumNum &&
		a.InputPremiumDenom == b.InputPremiumDenom &&
		a.OutputPremiumNum == b.OutputPremiumNum &&
		a.OutputPremiumDenom == b.OutputPremiumDenom
}
