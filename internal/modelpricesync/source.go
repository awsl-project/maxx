package modelpricesync

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

const DefaultSourceCode = "litellm"

const defaultLiteLLMSourceURL = "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json"

// Source describes one external model-price upstream. Each source owns its
// fetch URL configuration and converts upstream-specific rows into Maxx's
// unified domain.ModelPrice preview/apply shape.
type Source struct {
	Code       string
	Name       string
	DefaultURL string
	EnvURL     string
	Convert    func(modelID string, item LiteLLMModelPrice) (*domain.ModelPrice, bool)
}

var sources = map[string]Source{
	DefaultSourceCode: {
		Code:       DefaultSourceCode,
		Name:       "LiteLLM",
		DefaultURL: defaultLiteLLMSourceURL,
		EnvURL:     "MAXX_MODEL_PRICE_SYNC_SOURCE_URL",
		Convert:    ConvertLiteLLMModelPrice,
	},
}

// Resolve returns the configured source, defaulting to LiteLLM for backwards compatibility.
func Resolve(sourceCode string) (Source, error) {
	code := strings.TrimSpace(strings.ToLower(sourceCode))
	if code == "" {
		code = DefaultSourceCode
	}
	source, ok := sources[code]
	if !ok {
		return Source{}, fmt.Errorf("unsupported model price sync source %q", sourceCode)
	}
	return source, nil
}

// Fetch fetches and normalizes model prices from the requested source.
func Fetch(sourceCode string) ([]*domain.ModelPrice, Source, string, error) {
	source, err := Resolve(sourceCode)
	if err != nil {
		return nil, source, "", err
	}
	sourceURL := os.Getenv(source.EnvURL)
	if sourceURL == "" {
		sourceURL = source.DefaultURL
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(sourceURL)
	if err != nil {
		return nil, source, sourceURL, fmt.Errorf("fetch model price source %q: %w", source.Code, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, source, sourceURL, fmt.Errorf("fetch model price source %q: unexpected status %d", source.Code, resp.StatusCode)
	}

	var raw map[string]LiteLLMModelPrice
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, source, sourceURL, fmt.Errorf("decode model price source %q: %w", source.Code, err)
	}

	prices := make([]*domain.ModelPrice, 0, len(raw))
	for modelID, item := range raw {
		price, ok := source.Convert(modelID, item)
		if ok {
			prices = append(prices, price)
		}
	}
	sort.Slice(prices, func(i, j int) bool { return prices[i].ModelID < prices[j].ModelID })
	return prices, source, sourceURL, nil
}

// LiteLLMModelPrice is the LiteLLM model_prices_and_context_window.json row shape.
type LiteLLMModelPrice struct {
	Mode                                      string   `json:"mode"`
	LitellmProvider                           string   `json:"litellm_provider"`
	InputCostPerToken                         *float64 `json:"input_cost_per_token"`
	OutputCostPerToken                        *float64 `json:"output_cost_per_token"`
	CacheReadInputTokenCost                   *float64 `json:"cache_read_input_token_cost"`
	CacheCreationInputTokenCost               *float64 `json:"cache_creation_input_token_cost"`
	CacheCreationInputTokenCostAbove1hr       *float64 `json:"cache_creation_input_token_cost_above_1hr"`
	InputCostPerImageToken                    *float64 `json:"input_cost_per_image_token"`
	OutputCostPerImageToken                   *float64 `json:"output_cost_per_image_token"`
	InputCostPerTokenAbove200kTokens          *float64 `json:"input_cost_per_token_above_200k_tokens"`
	OutputCostPerTokenAbove200kTokens         *float64 `json:"output_cost_per_token_above_200k_tokens"`
	CacheReadInputTokenCostAbove200kTokens    *float64 `json:"cache_read_input_token_cost_above_200k_tokens"`
	CacheCreationInputTokenCostAbove200kToken *float64 `json:"cache_creation_input_token_cost_above_200k_tokens"`
}

func ConvertLiteLLMModelPrice(modelID string, item LiteLLMModelPrice) (*domain.ModelPrice, bool) {
	if modelID == "" || strings.HasPrefix(modelID, "sample_") {
		return nil, false
	}

	input := costPerTokenToMicroPerM(item.InputCostPerToken)
	output := costPerTokenToMicroPerM(item.OutputCostPerToken)
	imageInput := costPerTokenToMicroPerM(item.InputCostPerImageToken)
	imageOutput := costPerTokenToMicroPerM(item.OutputCostPerImageToken)
	if input == 0 && output == 0 && imageInput == 0 && imageOutput == 0 {
		return nil, false
	}

	price := &domain.ModelPrice{
		ModelID:                modelID,
		InputPriceMicro:        input,
		OutputPriceMicro:       output,
		CacheReadPriceMicro:    costPerTokenToMicroPerM(item.CacheReadInputTokenCost),
		Cache5mWritePriceMicro: costPerTokenToMicroPerM(item.CacheCreationInputTokenCost),
		Cache1hWritePriceMicro: costPerTokenToMicroPerM(item.CacheCreationInputTokenCostAbove1hr),
		ImageInputPriceMicro:   imageInput,
		ImageOutputPriceMicro:  imageOutput,
	}
	if price.Cache1hWritePriceMicro == 0 {
		price.Cache1hWritePriceMicro = price.Cache5mWritePriceMicro
	}

	inputAbove := costPerTokenToMicroPerM(item.InputCostPerTokenAbove200kTokens)
	outputAbove := costPerTokenToMicroPerM(item.OutputCostPerTokenAbove200kTokens)
	if inputAbove > input && outputAbove > output && input > 0 && output > 0 {
		price.Has1MContext = true
		price.Context1MThreshold = 200000
		price.InputPremiumNum, price.InputPremiumDenom = ratio(inputAbove, input)
		price.OutputPremiumNum, price.OutputPremiumDenom = ratio(outputAbove, output)
	}

	return price, true
}

func costPerTokenToMicroPerM(v *float64) uint64 {
	if v == nil || *v <= 0 {
		return 0
	}
	return uint64(math.Round(*v * 1_000_000_000_000))
}

func ratio(numerator, denominator uint64) (uint64, uint64) {
	if denominator == 0 || numerator == 0 {
		return 0, 0
	}
	g := gcd(numerator, denominator)
	return numerator / g, denominator / g
}

func gcd(a, b uint64) uint64 {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}
