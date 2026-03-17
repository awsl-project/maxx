package domain

import "time"

// BackupVersion current backup format version
const BackupVersion = "1.0"

// BackupFile represents the complete backup structure
type BackupFile struct {
	Version    string     `json:"version"`
	ExportedAt time.Time  `json:"exportedAt"`
	AppVersion string     `json:"appVersion"`
	Data       BackupData `json:"data"`
}

// BackupData contains all exportable entities
type BackupData struct {
	SystemSettings    []BackupSystemSetting   `json:"systemSettings,omitempty"`
	Providers         []BackupProvider        `json:"providers,omitempty"`
	Projects          []BackupProject         `json:"projects,omitempty"`
	RetryConfigs      []BackupRetryConfig     `json:"retryConfigs,omitempty"`
	Routes            []BackupRoute           `json:"routes,omitempty"`
	RoutingStrategies []BackupRoutingStrategy `json:"routingStrategies,omitempty"`
	APITokens         []BackupAPIToken        `json:"apiTokens,omitempty"`
	ModelMappings     []BackupModelMapping    `json:"modelMappings,omitempty"`
	ModelPrices       []BackupModelPrice      `json:"modelPrices,omitempty"`
}

// BackupSystemSetting represents a system setting for backup
type BackupSystemSetting struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// BackupProvider represents a provider for backup (using name as identifier)
type BackupProvider struct {
	Name                 string          `json:"name"`
	Type                 string          `json:"type"`
	Logo                 string          `json:"logo,omitempty"`
	Config               *ProviderConfig `json:"config,omitempty"`
	SupportedClientTypes []ClientType    `json:"supportedClientTypes,omitempty"`
	SupportModels        []string        `json:"supportModels,omitempty"`
}

// BackupProject represents a project for backup (using slug as identifier)
type BackupProject struct {
	Name                string       `json:"name"`
	Slug                string       `json:"slug"`
	EnabledCustomRoutes []ClientType `json:"enabledCustomRoutes,omitempty"`
}

// BackupRetryConfig represents a retry config for backup
type BackupRetryConfig struct {
	Name              string  `json:"name"`
	IsDefault         bool    `json:"isDefault"`
	MaxRetries        int     `json:"maxRetries"`
	InitialIntervalMs int64   `json:"initialIntervalMs"`
	BackoffRate       float64 `json:"backoffRate"`
	MaxIntervalMs     int64   `json:"maxIntervalMs"`
}

// BackupRoute represents a route for backup (using names instead of IDs)
type BackupRoute struct {
	IsEnabled       bool       `json:"isEnabled"`
	IsNative        bool       `json:"isNative"`
	ProjectSlug     string     `json:"projectSlug"` // empty = global
	ClientType      ClientType `json:"clientType"`
	ProviderName    string     `json:"providerName"`
	Position        int        `json:"position"`
	RetryConfigName string     `json:"retryConfigName"` // empty = default
}

// BackupRoutingStrategy represents a routing strategy for backup
type BackupRoutingStrategy struct {
	ProjectSlug string                 `json:"projectSlug"` // empty = global
	Type        RoutingStrategyType    `json:"type"`
	Config      *RoutingStrategyConfig `json:"config,omitempty"`
}

// BackupAPIToken represents an API token for backup
type BackupAPIToken struct {
	Name        string     `json:"name"`
	Token       string     `json:"token,omitempty"`       // plaintext token for import
	TokenPrefix string     `json:"tokenPrefix,omitempty"` // display prefix
	Description string     `json:"description"`
	ProjectSlug string     `json:"projectSlug"` // empty = global
	IsEnabled   bool       `json:"isEnabled"`
	DevMode     bool       `json:"devMode"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"`
}

// BackupModelMapping represents a model mapping for backup
type BackupModelMapping struct {
	Scope        ModelMappingScope `json:"scope"`
	ClientType   ClientType        `json:"clientType,omitempty"`
	ProviderType string            `json:"providerType,omitempty"`
	ProviderName string            `json:"providerName,omitempty"` // instead of ProviderID
	ProjectSlug  string            `json:"projectSlug,omitempty"`  // instead of ProjectID
	RouteName    string            `json:"routeName,omitempty"`    // instead of RouteID (providerName:clientType:projectSlug)
	APITokenName string            `json:"apiTokenName,omitempty"` // instead of APITokenID
	Pattern      string            `json:"pattern"`
	Target       string            `json:"target"`
	Priority     int               `json:"priority"`
}

// BackupModelPrice represents a model price for backup
type BackupModelPrice struct {
	ModelID                string `json:"modelId"`
	InputPriceMicro        uint64 `json:"inputPriceMicro"`
	OutputPriceMicro       uint64 `json:"outputPriceMicro"`
	CacheReadPriceMicro    uint64 `json:"cacheReadPriceMicro"`
	Cache5mWritePriceMicro uint64 `json:"cache5mWritePriceMicro"`
	Cache1hWritePriceMicro uint64 `json:"cache1hWritePriceMicro"`
	Has1MContext           bool   `json:"has1mContext"`
	Context1MThreshold     uint64 `json:"context1mThreshold"`
	InputPremiumNum        uint64 `json:"inputPremiumNum"`
	InputPremiumDenom      uint64 `json:"inputPremiumDenom"`
	OutputPremiumNum       uint64 `json:"outputPremiumNum"`
	OutputPremiumDenom     uint64 `json:"outputPremiumDenom"`
}

// ImportOptions defines options for import operation
type ImportOptions struct {
	ConflictStrategy string `json:"conflictStrategy"` // "skip", "overwrite", "error"
	DryRun           bool   `json:"dryRun"`
}

// ImportSummary contains counts for a single entity type
type ImportSummary struct {
	Imported int `json:"imported"`
	Skipped  int `json:"skipped"`
	Updated  int `json:"updated"`
}

// ImportResult contains the result of an import operation
type ImportResult struct {
	Success  bool                     `json:"success"`
	Summary  map[string]ImportSummary `json:"summary"`
	Errors   []string                 `json:"errors"`
	Warnings []string                 `json:"warnings"`
}

// NewImportResult creates a new ImportResult with initialized fields
func NewImportResult() *ImportResult {
	return &ImportResult{
		Success:  true,
		Summary:  make(map[string]ImportSummary),
		Errors:   []string{},
		Warnings: []string{},
	}
}
