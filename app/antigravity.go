package app

import (
	"context"
	"fmt"
	"time"

	"github.com/awsl-project/maxx/internal/adapter/provider/antigravity"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/event"
	"github.com/awsl-project/maxx/internal/repository"
	"github.com/awsl-project/maxx/internal/service"
)

type AntigravityService struct {
	quotaRepo    repository.AntigravityQuotaRepository
	oauthManager *antigravity.OAuthManager
	adminService *service.AdminService
	broadcaster  event.Broadcaster
}

func NewAntigravityService(quotaRepo repository.AntigravityQuotaRepository, broadcaster event.Broadcaster, adminService *service.AdminService) *AntigravityService {
	return &AntigravityService{
		quotaRepo:    quotaRepo,
		oauthManager: antigravity.NewOAuthManager(broadcaster),
		adminService: adminService,
		broadcaster:  broadcaster,
	}
}

type TokenValidationResult = antigravity.TokenValidationResult
type QuotaData = antigravity.QuotaData
type OAuthResult = antigravity.OAuthResult

func (s *AntigravityService) ValidateToken(ctx context.Context, refreshToken string) (*TokenValidationResult, error) {
	result, err := antigravity.ValidateRefreshToken(ctx, refreshToken)
	if err != nil {
		return nil, err
	}

	if result.Valid && result.UserInfo != nil && result.UserInfo.Email != "" {
		s.saveQuotaToDB(result.UserInfo.Email, result.UserInfo.Name, result.UserInfo.Picture, result.ProjectID, result.Quota)
	}

	return result, nil
}

func (s *AntigravityService) ValidateTokens(ctx context.Context, tokens []string) []*TokenValidationResult {
	results := antigravity.BatchValidateRefreshTokens(ctx, tokens)

	for _, result := range results {
		if result.Valid && result.UserInfo != nil && result.UserInfo.Email != "" {
			s.saveQuotaToDB(result.UserInfo.Email, result.UserInfo.Name, result.UserInfo.Picture, result.ProjectID, result.Quota)
		}
	}

	return results
}

func (s *AntigravityService) ValidateTokenText(ctx context.Context, tokenText string) []*TokenValidationResult {
	tokens := antigravity.ParseRefreshTokens(tokenText)
	return s.ValidateTokens(ctx, tokens)
}

func (s *AntigravityService) GetProviderQuota(ctx context.Context, providerID uint64, forceRefresh bool) (*QuotaData, error) {
	provider, err := s.adminService.GetProvider(providerID)
	if err != nil {
		return nil, err
	}

	if provider.Type != "antigravity" || provider.Config == nil || provider.Config.Antigravity == nil {
		return nil, fmt.Errorf("not an antigravity provider")
	}

	config := provider.Config.Antigravity
	quota, err := antigravity.FetchQuotaForProvider(ctx, config.RefreshToken, config.ProjectID)
	if err != nil {
		return nil, err
	}

	return quota, nil
}

func (s *AntigravityService) StartOAuth(ctx context.Context, redirectURI string) (map[string]interface{}, error) {
	state, err := s.oauthManager.GenerateState()
	if err != nil {
		return nil, err
	}

	s.oauthManager.CreateSession(state)

	authURL := antigravity.GetAuthURL(redirectURI, state)

	return map[string]interface{}{
		"authURL": authURL,
		"state":   state,
	}, nil
}

func (s *AntigravityService) saveQuotaToDB(email, name, picture, projectID string, quota *antigravity.QuotaData) {
	if s.quotaRepo == nil || email == "" {
		return
	}

	var models []domain.AntigravityModelQuota
	var subscriptionTier string
	var isForbidden bool
	var lastUpdated int64

	if quota != nil {
		models = make([]domain.AntigravityModelQuota, len(quota.Models))
		for i, m := range quota.Models {
			models[i] = domain.AntigravityModelQuota{
				Name:       m.Name,
				Percentage: m.Percentage,
				ResetTime:  m.ResetTime,
			}
		}
		subscriptionTier = quota.SubscriptionTier
		isForbidden = quota.IsForbidden
		lastUpdated = quota.LastUpdated
	} else {
		lastUpdated = time.Now().Unix()
	}

	domainQuota := &domain.AntigravityQuota{
		Email:            email,
		Name:             name,
		Picture:          picture,
		ProjectID:        projectID,
		SubscriptionTier: subscriptionTier,
		IsForbidden:      isForbidden,
		Models:           models,
		LastUpdated:      lastUpdated,
	}

	_ = s.quotaRepo.Upsert(domainQuota)
}
