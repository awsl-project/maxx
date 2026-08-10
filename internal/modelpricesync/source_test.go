package modelpricesync

import "testing"

func TestResolveDefaultsToLiteLLM(t *testing.T) {
	source, err := Resolve("")
	if err != nil {
		t.Fatalf("Resolve empty source: %v", err)
	}
	if source.Code != DefaultSourceCode {
		t.Fatalf("source.Code = %q, want %q", source.Code, DefaultSourceCode)
	}
}

func TestConvertLiteLLMModelPrice(t *testing.T) {
	input := 0.000003
	output := 0.000015
	cacheRead := 0.0000003
	cacheWrite := 0.00000375
	inputAbove := 0.000006
	outputAbove := 0.0000225

	price, ok := ConvertLiteLLMModelPrice("sync-model", LiteLLMModelPrice{
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
	if price.ModelID != "sync-model" || price.InputPriceMicro != 3_000_000 || price.OutputPriceMicro != 15_000_000 {
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
