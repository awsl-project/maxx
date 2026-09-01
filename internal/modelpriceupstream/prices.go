package modelpriceupstream

import (
	"context"

	"github.com/awsl-project/maxx/internal/domain"
)

// Result returns formatted upstream model prices.
type Result struct {
	Source    string               `json:"source"`
	SourceURL string               `json:"sourceUrl"`
	Total     int                  `json:"total"`
	Prices    []*domain.ModelPrice `json:"prices"`
}

// List fetches source prices and returns them normalized into Maxx's model price shape.
func List(ctx context.Context, sourceCode string) (*Result, error) {
	prices, source, sourceURL, err := Fetch(ctx, sourceCode)
	if err != nil {
		return nil, err
	}
	return &Result{Source: source.Code, SourceURL: sourceURL, Total: len(prices), Prices: prices}, nil
}
