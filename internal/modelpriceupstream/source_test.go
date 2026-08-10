package modelpriceupstream

import (
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
)

func TestResolveDefaultsToLiteLLM(t *testing.T) {
	source, err := Resolve("")
	if err != nil {
		t.Fatalf("Resolve empty source: %v", err)
	}
	if source.Code() != DefaultSourceCode {
		t.Fatalf("source.Code() = %q, want %q", source.Code(), DefaultSourceCode)
	}
}

type fakeSource struct{}

func (fakeSource) Code() string { return "fake" }
func (fakeSource) Name() string { return "Fake" }
func (fakeSource) Fetch() ([]*domain.ModelPrice, string, error) {
	return []*domain.ModelPrice{{ModelID: "fake-model", InputPriceMicro: 1}}, "memory://fake", nil
}

func TestRegisterSupportsIndependentSourceImplementations(t *testing.T) {
	previous, existed := sources["fake"]
	t.Cleanup(func() {
		if existed {
			sources["fake"] = previous
		} else {
			delete(sources, "fake")
		}
	})

	if err := Register(fakeSource{}); err != nil {
		t.Fatalf("Register fake source: %v", err)
	}

	prices, source, sourceURL, err := Fetch("fake")
	if err != nil {
		t.Fatalf("Fetch fake source: %v", err)
	}
	if source.Code != "fake" || source.Name != "Fake" || sourceURL != "memory://fake" {
		t.Fatalf("unexpected source metadata: source=%+v sourceURL=%q", source, sourceURL)
	}
	if len(prices) != 1 || prices[0].ModelID != "fake-model" {
		t.Fatalf("unexpected fake prices: %+v", prices)
	}
}

func TestConvertLiteLLMModelPrice(t *testing.T) {
	input := 0.000003
	output := 0.000015
	cacheRead := 0.0000003
	cacheWrite := 0.00000375
	inputAbove := 0.000006
	outputAbove := 0.0000225

	price, ok := ConvertLiteLLMModelPrice("upstream-model", LiteLLMModelPrice{
		InputCostPerToken:                 &input,
		OutputCostPerToken:                &output,
		CacheReadInputTokenCost:           &cacheRead,
		CacheCreationInputTokenCost:       &cacheWrite,
		InputCostPerTokenAbove200kTokens:  &inputAbove,
		OutputCostPerTokenAbove200kTokens: &outputAbove,
	})
	if !ok || price == nil {
		t.Fatal("expected LiteLLM row to convert")
	}
	if price.ModelID != "upstream-model" || price.InputPriceMicro != 3_000_000 || price.OutputPriceMicro != 15_000_000 {
		t.Fatalf("unexpected converted price: %+v", price)
	}
	if price.CacheReadPriceMicro != 300_000 || price.Cache5mWritePriceMicro != 3_750_000 || price.Cache1hWritePriceMicro != 3_750_000 {
		t.Fatalf("unexpected cache prices: %+v", price)
	}
	if !price.Has1MContext || price.Context1MThreshold != 200000 {
		t.Fatalf("expected 1M context pricing: %+v", price)
	}
}

func TestConvertLiteLLMModelPriceSkipsSamplesAndEmptyPrices(t *testing.T) {
	if price, ok := ConvertLiteLLMModelPrice("sample_spec", LiteLLMModelPrice{}); ok || price != nil {
		t.Fatalf("expected sample row to be skipped, got ok=%v price=%+v", ok, price)
	}
	if price, ok := ConvertLiteLLMModelPrice("free-model", LiteLLMModelPrice{}); ok || price != nil {
		t.Fatalf("expected zero-price row to be skipped, got ok=%v price=%+v", ok, price)
	}
}
