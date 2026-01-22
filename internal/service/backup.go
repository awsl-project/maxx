package service

import (
	"fmt"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository"
	"github.com/awsl-project/maxx/internal/version"
)

// BackupService handles backup export and import operations
type BackupService struct {
	providerRepo        repository.ProviderRepository
	routeRepo           repository.RouteRepository
	projectRepo         repository.ProjectRepository
	retryConfigRepo     repository.RetryConfigRepository
	routingStrategyRepo repository.RoutingStrategyRepository
	settingRepo         repository.SystemSettingRepository
	apiTokenRepo        repository.APITokenRepository
	modelMappingRepo    repository.ModelMappingRepository
	adapterRefresher    ProviderAdapterRefresher
}

// NewBackupService creates a new backup service
func NewBackupService(
	providerRepo repository.ProviderRepository,
	routeRepo repository.RouteRepository,
	projectRepo repository.ProjectRepository,
	retryConfigRepo repository.RetryConfigRepository,
	routingStrategyRepo repository.RoutingStrategyRepository,
	settingRepo repository.SystemSettingRepository,
	apiTokenRepo repository.APITokenRepository,
	modelMappingRepo repository.ModelMappingRepository,
	adapterRefresher ProviderAdapterRefresher,
) *BackupService {
	return &BackupService{
		providerRepo:        providerRepo,
		routeRepo:           routeRepo,
		projectRepo:         projectRepo,
		retryConfigRepo:     retryConfigRepo,
		routingStrategyRepo: routingStrategyRepo,
		settingRepo:         settingRepo,
		apiTokenRepo:        apiTokenRepo,
		modelMappingRepo:    modelMappingRepo,
		adapterRefresher:    adapterRefresher,
	}
}

// importContext holds ID mappings during import
type importContext struct {
	providerNameToID    map[string]uint64
	projectSlugToID     map[string]uint64
	retryConfigNameToID map[string]uint64
	apiTokenNameToID    map[string]uint64
	// routeKey format: "projectSlug:clientType:providerName"
	routeKeyToID map[string]uint64
}

func newImportContext() *importContext {
	return &importContext{
		providerNameToID:    make(map[string]uint64),
		projectSlugToID:     make(map[string]uint64),
		retryConfigNameToID: make(map[string]uint64),
		apiTokenNameToID:    make(map[string]uint64),
		routeKeyToID:        make(map[string]uint64),
	}
}

// Export exports all configuration data to a backup file
func (s *BackupService) Export() (*domain.BackupFile, error) {
	backup := &domain.BackupFile{
		Version:    domain.BackupVersion,
		ExportedAt: time.Now(),
		AppVersion: version.Version,
	}

	// Build lookup maps for ID to name conversion
	providerIDToName := make(map[uint64]string)
	projectIDToSlug := make(map[uint64]string)
	retryConfigIDToName := make(map[uint64]string)
	apiTokenIDToName := make(map[uint64]string)

	// 1. Export SystemSettings
	settings, err := s.settingRepo.GetAll()
	if err != nil {
		return nil, fmt.Errorf("failed to export settings: %w", err)
	}
	for _, setting := range settings {
		backup.Data.SystemSettings = append(backup.Data.SystemSettings, domain.BackupSystemSetting{
			Key:   setting.Key,
			Value: setting.Value,
		})
	}

	// 2. Export Providers
	providers, err := s.providerRepo.List()
	if err != nil {
		return nil, fmt.Errorf("failed to export providers: %w", err)
	}
	for _, p := range providers {
		providerIDToName[p.ID] = p.Name
		backup.Data.Providers = append(backup.Data.Providers, domain.BackupProvider{
			Name:                 p.Name,
			Type:                 p.Type,
			Config:               p.Config,
			SupportedClientTypes: p.SupportedClientTypes,
			SupportModels:        p.SupportModels,
		})
	}

	// 3. Export Projects
	projects, err := s.projectRepo.List()
	if err != nil {
		return nil, fmt.Errorf("failed to export projects: %w", err)
	}
	for _, p := range projects {
		projectIDToSlug[p.ID] = p.Slug
		backup.Data.Projects = append(backup.Data.Projects, domain.BackupProject{
			Name:                p.Name,
			Slug:                p.Slug,
			EnabledCustomRoutes: p.EnabledCustomRoutes,
		})
	}

	// 4. Export RetryConfigs
	retryConfigs, err := s.retryConfigRepo.List()
	if err != nil {
		return nil, fmt.Errorf("failed to export retry configs: %w", err)
	}
	for _, rc := range retryConfigs {
		retryConfigIDToName[rc.ID] = rc.Name
		backup.Data.RetryConfigs = append(backup.Data.RetryConfigs, domain.BackupRetryConfig{
			Name:              rc.Name,
			IsDefault:         rc.IsDefault,
			MaxRetries:        rc.MaxRetries,
			InitialIntervalMs: rc.InitialInterval.Milliseconds(),
			BackoffRate:       rc.BackoffRate,
			MaxIntervalMs:     rc.MaxInterval.Milliseconds(),
		})
	}

	// 5. Export RoutingStrategies
	strategies, err := s.routingStrategyRepo.List()
	if err != nil {
		return nil, fmt.Errorf("failed to export routing strategies: %w", err)
	}
	for _, rs := range strategies {
		backup.Data.RoutingStrategies = append(backup.Data.RoutingStrategies, domain.BackupRoutingStrategy{
			ProjectSlug: projectIDToSlug[rs.ProjectID],
			Type:        rs.Type,
			Config:      rs.Config,
		})
	}

	// 6. Export Routes
	routes, err := s.routeRepo.List()
	if err != nil {
		return nil, fmt.Errorf("failed to export routes: %w", err)
	}
	for _, r := range routes {
		backup.Data.Routes = append(backup.Data.Routes, domain.BackupRoute{
			IsEnabled:       r.IsEnabled,
			IsNative:        r.IsNative,
			ProjectSlug:     projectIDToSlug[r.ProjectID],
			ClientType:      r.ClientType,
			ProviderName:    providerIDToName[r.ProviderID],
			Position:        r.Position,
			RetryConfigName: retryConfigIDToName[r.RetryConfigID],
		})
	}

	// 7. Export APITokens (without token value)
	tokens, err := s.apiTokenRepo.List()
	if err != nil {
		return nil, fmt.Errorf("failed to export api tokens: %w", err)
	}
	for _, t := range tokens {
		apiTokenIDToName[t.ID] = t.Name
		backup.Data.APITokens = append(backup.Data.APITokens, domain.BackupAPIToken{
			Name:        t.Name,
			Description: t.Description,
			ProjectSlug: projectIDToSlug[t.ProjectID],
			IsEnabled:   t.IsEnabled,
			ExpiresAt:   t.ExpiresAt,
		})
	}

	// 8. Export ModelMappings
	mappings, err := s.modelMappingRepo.List()
	if err != nil {
		return nil, fmt.Errorf("failed to export model mappings: %w", err)
	}
	for _, m := range mappings {
		bm := domain.BackupModelMapping{
			Scope:        m.Scope,
			ClientType:   m.ClientType,
			ProviderType: m.ProviderType,
			Pattern:      m.Pattern,
			Target:       m.Target,
			Priority:     m.Priority,
		}
		// Convert IDs to names
		if m.ProviderID != 0 {
			bm.ProviderName = providerIDToName[m.ProviderID]
		}
		if m.ProjectID != 0 {
			bm.ProjectSlug = projectIDToSlug[m.ProjectID]
		}
		if m.APITokenID != 0 {
			bm.APITokenName = apiTokenIDToName[m.APITokenID]
		}
		// Route reference: combine identifiers
		if m.RouteID != 0 {
			// Find the route to get its composite key
			for _, r := range routes {
				if r.ID == m.RouteID {
					bm.RouteName = fmt.Sprintf("%s:%s:%s",
						providerIDToName[r.ProviderID],
						r.ClientType,
						projectIDToSlug[r.ProjectID])
					break
				}
			}
		}
		backup.Data.ModelMappings = append(backup.Data.ModelMappings, bm)
	}

	return backup, nil
}

// Import imports configuration data from a backup file
func (s *BackupService) Import(backup *domain.BackupFile, opts domain.ImportOptions) (*domain.ImportResult, error) {
	// Version check
	if backup.Version != domain.BackupVersion {
		return nil, fmt.Errorf("unsupported backup version: %s (expected %s)", backup.Version, domain.BackupVersion)
	}

	result := domain.NewImportResult()
	ctx := newImportContext()

	// Load existing data for conflict detection and ID mapping
	if err := s.loadExistingMappings(ctx); err != nil {
		return nil, fmt.Errorf("failed to load existing data: %w", err)
	}

	// Import in dependency order
	// 1. SystemSettings (no dependencies)
	s.importSystemSettings(backup.Data.SystemSettings, opts, result)

	// 2. RetryConfigs (no dependencies)
	s.importRetryConfigs(backup.Data.RetryConfigs, opts, result, ctx)

	// 3. Providers (no dependencies)
	s.importProviders(backup.Data.Providers, opts, result, ctx)

	// 4. Projects (no dependencies)
	s.importProjects(backup.Data.Projects, opts, result, ctx)

	// 5. RoutingStrategies (depends on Projects)
	s.importRoutingStrategies(backup.Data.RoutingStrategies, opts, result, ctx)

	// 6. Routes (depends on Providers, Projects, RetryConfigs)
	s.importRoutes(backup.Data.Routes, opts, result, ctx)

	// 7. APITokens (depends on Projects)
	s.importAPITokens(backup.Data.APITokens, opts, result, ctx)

	// 8. ModelMappings (depends on Providers, Projects, Routes, APITokens)
	s.importModelMappings(backup.Data.ModelMappings, opts, result, ctx)

	return result, nil
}

// loadExistingMappings loads existing data and populates the import context
func (s *BackupService) loadExistingMappings(ctx *importContext) error {
	// Load providers
	providers, err := s.providerRepo.List()
	if err != nil {
		return err
	}
	for _, p := range providers {
		ctx.providerNameToID[p.Name] = p.ID
	}

	// Load projects
	projects, err := s.projectRepo.List()
	if err != nil {
		return err
	}
	for _, p := range projects {
		ctx.projectSlugToID[p.Slug] = p.ID
	}

	// Load retry configs
	retryConfigs, err := s.retryConfigRepo.List()
	if err != nil {
		return err
	}
	for _, rc := range retryConfigs {
		ctx.retryConfigNameToID[rc.Name] = rc.ID
	}

	// Load API tokens
	tokens, err := s.apiTokenRepo.List()
	if err != nil {
		return err
	}
	for _, t := range tokens {
		ctx.apiTokenNameToID[t.Name] = t.ID
	}

	// Load routes
	routes, err := s.routeRepo.List()
	if err != nil {
		return err
	}
	for _, r := range routes {
		providerName := ""
		for name, id := range ctx.providerNameToID {
			if id == r.ProviderID {
				providerName = name
				break
			}
		}
		projectSlug := ""
		for slug, id := range ctx.projectSlugToID {
			if id == r.ProjectID {
				projectSlug = slug
				break
			}
		}
		key := fmt.Sprintf("%s:%s:%s", providerName, r.ClientType, projectSlug)
		ctx.routeKeyToID[key] = r.ID
	}

	return nil
}

func (s *BackupService) importSystemSettings(settings []domain.BackupSystemSetting, opts domain.ImportOptions, result *domain.ImportResult) {
	summary := domain.ImportSummary{}

	for _, bs := range settings {
		existing, _ := s.settingRepo.Get(bs.Key)
		if existing != "" {
			switch opts.ConflictStrategy {
			case "skip", "":
				summary.Skipped++
				continue
			case "overwrite":
				if !opts.DryRun {
					s.settingRepo.Set(bs.Key, bs.Value)
				}
				summary.Updated++
			case "error":
				result.Success = false
				result.Errors = append(result.Errors, fmt.Sprintf("SystemSetting conflict: key '%s' already exists", bs.Key))
				return
			}
		} else {
			if !opts.DryRun {
				s.settingRepo.Set(bs.Key, bs.Value)
			}
			summary.Imported++
		}
	}

	result.Summary["systemSettings"] = summary
}

func (s *BackupService) importRetryConfigs(configs []domain.BackupRetryConfig, opts domain.ImportOptions, result *domain.ImportResult, ctx *importContext) {
	summary := domain.ImportSummary{}

	for _, bc := range configs {
		if _, exists := ctx.retryConfigNameToID[bc.Name]; exists {
			switch opts.ConflictStrategy {
			case "skip", "":
				summary.Skipped++
				continue
			case "overwrite":
				// For now, skip overwrite of retry configs (complex due to references)
				summary.Skipped++
				result.Warnings = append(result.Warnings, fmt.Sprintf("RetryConfig '%s' overwrite not supported, skipped", bc.Name))
				continue
			case "error":
				result.Success = false
				result.Errors = append(result.Errors, fmt.Sprintf("RetryConfig conflict: '%s' already exists", bc.Name))
				return
			}
		}

		rc := &domain.RetryConfig{
			Name:            bc.Name,
			IsDefault:       bc.IsDefault,
			MaxRetries:      bc.MaxRetries,
			InitialInterval: time.Duration(bc.InitialIntervalMs) * time.Millisecond,
			BackoffRate:     bc.BackoffRate,
			MaxInterval:     time.Duration(bc.MaxIntervalMs) * time.Millisecond,
		}

		if !opts.DryRun {
			if err := s.retryConfigRepo.Create(rc); err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to import RetryConfig '%s': %v", bc.Name, err))
				continue
			}
			ctx.retryConfigNameToID[bc.Name] = rc.ID
		}
		summary.Imported++
	}

	result.Summary["retryConfigs"] = summary
}

func (s *BackupService) importProviders(providers []domain.BackupProvider, opts domain.ImportOptions, result *domain.ImportResult, ctx *importContext) {
	summary := domain.ImportSummary{}

	for _, bp := range providers {
		if _, exists := ctx.providerNameToID[bp.Name]; exists {
			switch opts.ConflictStrategy {
			case "skip", "":
				summary.Skipped++
				continue
			case "overwrite":
				// Skip overwrite for providers (complex due to adapter refresh)
				summary.Skipped++
				result.Warnings = append(result.Warnings, fmt.Sprintf("Provider '%s' overwrite not supported, skipped", bp.Name))
				continue
			case "error":
				result.Success = false
				result.Errors = append(result.Errors, fmt.Sprintf("Provider conflict: '%s' already exists", bp.Name))
				return
			}
		}

		p := &domain.Provider{
			Name:                 bp.Name,
			Type:                 bp.Type,
			Config:               bp.Config,
			SupportedClientTypes: bp.SupportedClientTypes,
			SupportModels:        bp.SupportModels,
		}

		if !opts.DryRun {
			if err := s.providerRepo.Create(p); err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to import Provider '%s': %v", bp.Name, err))
				continue
			}
			ctx.providerNameToID[bp.Name] = p.ID
			// Refresh adapter
			if s.adapterRefresher != nil {
				s.adapterRefresher.RefreshAdapter(p)
			}
		}
		summary.Imported++
	}

	result.Summary["providers"] = summary
}

func (s *BackupService) importProjects(projects []domain.BackupProject, opts domain.ImportOptions, result *domain.ImportResult, ctx *importContext) {
	summary := domain.ImportSummary{}

	for _, bp := range projects {
		if _, exists := ctx.projectSlugToID[bp.Slug]; exists {
			switch opts.ConflictStrategy {
			case "skip", "":
				summary.Skipped++
				continue
			case "overwrite":
				summary.Skipped++
				result.Warnings = append(result.Warnings, fmt.Sprintf("Project '%s' overwrite not supported, skipped", bp.Slug))
				continue
			case "error":
				result.Success = false
				result.Errors = append(result.Errors, fmt.Sprintf("Project conflict: '%s' already exists", bp.Slug))
				return
			}
		}

		p := &domain.Project{
			Name:                bp.Name,
			Slug:                bp.Slug,
			EnabledCustomRoutes: bp.EnabledCustomRoutes,
		}

		if !opts.DryRun {
			if err := s.projectRepo.Create(p); err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to import Project '%s': %v", bp.Slug, err))
				continue
			}
			ctx.projectSlugToID[bp.Slug] = p.ID
		}
		summary.Imported++
	}

	result.Summary["projects"] = summary
}

func (s *BackupService) importRoutingStrategies(strategies []domain.BackupRoutingStrategy, opts domain.ImportOptions, result *domain.ImportResult, ctx *importContext) {
	summary := domain.ImportSummary{}

	for _, bs := range strategies {
		var projectID uint64
		if bs.ProjectSlug != "" {
			var ok bool
			projectID, ok = ctx.projectSlugToID[bs.ProjectSlug]
			if !ok {
				result.Warnings = append(result.Warnings, fmt.Sprintf("RoutingStrategy skipped: project '%s' not found", bs.ProjectSlug))
				summary.Skipped++
				continue
			}
		}

		// Check if strategy exists for this project
		existing, _ := s.routingStrategyRepo.GetByProjectID(projectID)
		if existing != nil {
			switch opts.ConflictStrategy {
			case "skip", "":
				summary.Skipped++
				continue
			case "overwrite":
				existing.Type = bs.Type
				existing.Config = bs.Config
				if !opts.DryRun {
					s.routingStrategyRepo.Update(existing)
				}
				summary.Updated++
				continue
			case "error":
				result.Success = false
				result.Errors = append(result.Errors, fmt.Sprintf("RoutingStrategy conflict for project '%s'", bs.ProjectSlug))
				return
			}
		}

		rs := &domain.RoutingStrategy{
			ProjectID: projectID,
			Type:      bs.Type,
			Config:    bs.Config,
		}

		if !opts.DryRun {
			if err := s.routingStrategyRepo.Create(rs); err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to import RoutingStrategy: %v", err))
				continue
			}
		}
		summary.Imported++
	}

	result.Summary["routingStrategies"] = summary
}

func (s *BackupService) importRoutes(routes []domain.BackupRoute, opts domain.ImportOptions, result *domain.ImportResult, ctx *importContext) {
	summary := domain.ImportSummary{}

	for _, br := range routes {
		// Resolve provider
		providerID, ok := ctx.providerNameToID[br.ProviderName]
		if !ok {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Route skipped: provider '%s' not found", br.ProviderName))
			summary.Skipped++
			continue
		}

		// Resolve project
		var projectID uint64
		if br.ProjectSlug != "" {
			projectID, ok = ctx.projectSlugToID[br.ProjectSlug]
			if !ok {
				result.Warnings = append(result.Warnings, fmt.Sprintf("Route skipped: project '%s' not found", br.ProjectSlug))
				summary.Skipped++
				continue
			}
		}

		// Resolve retry config
		var retryConfigID uint64
		if br.RetryConfigName != "" {
			retryConfigID = ctx.retryConfigNameToID[br.RetryConfigName]
		}

		// Check for existing route
		routeKey := fmt.Sprintf("%s:%s:%s", br.ProviderName, br.ClientType, br.ProjectSlug)
		if _, exists := ctx.routeKeyToID[routeKey]; exists {
			switch opts.ConflictStrategy {
			case "skip", "":
				summary.Skipped++
				continue
			case "overwrite":
				summary.Skipped++
				result.Warnings = append(result.Warnings, "Route overwrite not supported, skipped")
				continue
			case "error":
				result.Success = false
				result.Errors = append(result.Errors, "Route conflict: route already exists")
				return
			}
		}

		r := &domain.Route{
			IsEnabled:     br.IsEnabled,
			IsNative:      br.IsNative,
			ProjectID:     projectID,
			ClientType:    br.ClientType,
			ProviderID:    providerID,
			Position:      br.Position,
			RetryConfigID: retryConfigID,
		}

		if !opts.DryRun {
			if err := s.routeRepo.Create(r); err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to import Route: %v", err))
				continue
			}
			ctx.routeKeyToID[routeKey] = r.ID
		}
		summary.Imported++
	}

	result.Summary["routes"] = summary
}

func (s *BackupService) importAPITokens(tokens []domain.BackupAPIToken, opts domain.ImportOptions, result *domain.ImportResult, ctx *importContext) {
	summary := domain.ImportSummary{}

	for _, bt := range tokens {
		if _, exists := ctx.apiTokenNameToID[bt.Name]; exists {
			switch opts.ConflictStrategy {
			case "skip", "":
				summary.Skipped++
				continue
			case "overwrite":
				summary.Skipped++
				result.Warnings = append(result.Warnings, fmt.Sprintf("APIToken '%s' overwrite not supported, skipped", bt.Name))
				continue
			case "error":
				result.Success = false
				result.Errors = append(result.Errors, fmt.Sprintf("APIToken conflict: '%s' already exists", bt.Name))
				return
			}
		}

		// Resolve project
		var projectID uint64
		if bt.ProjectSlug != "" {
			var ok bool
			projectID, ok = ctx.projectSlugToID[bt.ProjectSlug]
			if !ok {
				result.Warnings = append(result.Warnings, fmt.Sprintf("APIToken '%s' skipped: project '%s' not found", bt.Name, bt.ProjectSlug))
				summary.Skipped++
				continue
			}
		}

		// Generate new token
		plain, prefix, err := generateAPIToken()
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to generate token for '%s': %v", bt.Name, err))
			continue
		}

		t := &domain.APIToken{
			Token:       plain,
			TokenPrefix: prefix,
			Name:        bt.Name,
			Description: bt.Description,
			ProjectID:   projectID,
			IsEnabled:   bt.IsEnabled,
			ExpiresAt:   bt.ExpiresAt,
		}

		if !opts.DryRun {
			if err := s.apiTokenRepo.Create(t); err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to import APIToken '%s': %v", bt.Name, err))
				continue
			}
			ctx.apiTokenNameToID[bt.Name] = t.ID
			result.Warnings = append(result.Warnings, fmt.Sprintf("APIToken '%s' created with new token: %s", bt.Name, plain))
		}
		summary.Imported++
	}

	result.Summary["apiTokens"] = summary
}

func (s *BackupService) importModelMappings(mappings []domain.BackupModelMapping, opts domain.ImportOptions, result *domain.ImportResult, ctx *importContext) {
	summary := domain.ImportSummary{}

	for _, bm := range mappings {
		// Resolve IDs
		var providerID, projectID, routeID, apiTokenID uint64

		if bm.ProviderName != "" {
			var ok bool
			providerID, ok = ctx.providerNameToID[bm.ProviderName]
			if !ok {
				result.Warnings = append(result.Warnings, fmt.Sprintf("ModelMapping skipped: provider '%s' not found", bm.ProviderName))
				summary.Skipped++
				continue
			}
		}

		if bm.ProjectSlug != "" {
			var ok bool
			projectID, ok = ctx.projectSlugToID[bm.ProjectSlug]
			if !ok {
				result.Warnings = append(result.Warnings, fmt.Sprintf("ModelMapping skipped: project '%s' not found", bm.ProjectSlug))
				summary.Skipped++
				continue
			}
		}

		if bm.RouteName != "" {
			var ok bool
			routeID, ok = ctx.routeKeyToID[bm.RouteName]
			if !ok {
				result.Warnings = append(result.Warnings, fmt.Sprintf("ModelMapping skipped: route '%s' not found", bm.RouteName))
				summary.Skipped++
				continue
			}
		}

		if bm.APITokenName != "" {
			var ok bool
			apiTokenID, ok = ctx.apiTokenNameToID[bm.APITokenName]
			if !ok {
				result.Warnings = append(result.Warnings, fmt.Sprintf("ModelMapping skipped: apiToken '%s' not found", bm.APITokenName))
				summary.Skipped++
				continue
			}
		}

		m := &domain.ModelMapping{
			Scope:        bm.Scope,
			ClientType:   bm.ClientType,
			ProviderType: bm.ProviderType,
			ProviderID:   providerID,
			ProjectID:    projectID,
			RouteID:      routeID,
			APITokenID:   apiTokenID,
			Pattern:      bm.Pattern,
			Target:       bm.Target,
			Priority:     bm.Priority,
		}

		if !opts.DryRun {
			if err := s.modelMappingRepo.Create(m); err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to import ModelMapping: %v", err))
				continue
			}
		}
		summary.Imported++
	}

	result.Summary["modelMappings"] = summary
}
