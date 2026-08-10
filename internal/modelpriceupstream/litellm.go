package modelpriceupstream

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

const defaultLiteLLMSourceURL = "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json"

// LiteLLMSource fetches LiteLLM's public model_prices_and_context_window.json table.
type LiteLLMSource struct {
	DefaultURL string
	EnvURL     string
	Client     *http.Client
}

func NewLiteLLMSource() *LiteLLMSource {
	return &LiteLLMSource{
		DefaultURL: defaultLiteLLMSourceURL,
		EnvURL:     "MAXX_MODEL_PRICE_SYNC_SOURCE_URL",
		Client:     &http.Client{Timeout: 30 * time.Second},
	}
}

func (s *LiteLLMSource) Code() string { return DefaultSourceCode }
func (s *LiteLLMSource) Name() string { return "LiteLLM" }

func (s *LiteLLMSource) Fetch() ([]*domain.ModelPrice, string, error) {
	sourceURL := os.Getenv(s.EnvURL)
	if sourceURL == "" {
		sourceURL = s.DefaultURL
	}

	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Get(sourceURL)
	if err != nil {
		return nil, sourceURL, fmt.Errorf("fetch model price source %q: %w", s.Code(), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, sourceURL, fmt.Errorf("fetch model price source %q: unexpected status %d", s.Code(), resp.StatusCode)
	}

	var raw map[string]LiteLLMModelPrice
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, sourceURL, fmt.Errorf("decode model price source %q: %w", s.Code(), err)
	}

	prices := make([]*domain.ModelPrice, 0, len(raw))
	for modelID, item := range raw {
		price, ok := ConvertLiteLLMModelPrice(modelID, item)
		if ok {
			prices = append(prices, price)
		}
	}
	return prices, sourceURL, nil
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
