package core

import (
	"context"
	"log"
	"time"

	"github.com/awsl-project/maxx/internal/cooldown"
	"github.com/awsl-project/maxx/internal/coordinator"
	"github.com/awsl-project/maxx/internal/repository/cached"
	"github.com/awsl-project/maxx/internal/sticky"
)

// CoordinatorComponents 是 SetupCoordinator 的输出。
// Cleanup 是收尾函数,顺序为 Unregister → Cancel → Close。
type CoordinatorComponents struct {
	Coordinator coordinator.Coordinator
	Config      coordinator.Config
	Ctx         context.Context
	Cancel      context.CancelFunc
	Cleanup     func()
}

// SetupCoordinator 装配 coordinator 子系统:构造 coordinator、注册心跳、
// 把 coordinator 注入 cooldown.Default()。
//
// 两条启动路径(cmd/maxx 和 desktop launcher)共用此函数,保证它们的
// distributed 行为完全一致。
//
// forceStandalone=true 时,无论环境变量怎么设都强制 standalone。Desktop
// 应用一律传 true:multi-instance 在桌面场景没有意义。
func SetupCoordinator(parentCtx context.Context, instanceID string, forceStandalone bool) (*CoordinatorComponents, error) {
	cfg := coordinator.ConfigFromEnv()
	if forceStandalone {
		if cfg.Mode != coordinator.ModeStandalone {
			log.Printf("[Coordinator] forcing standalone mode for desktop (env requested %s)", cfg.Mode)
		}
		cfg.Mode = coordinator.ModeStandalone
		cfg.RedisURL = ""
	}

	coordCtx, coordCancel := context.WithCancel(parentCtx)
	coord, coordCleanup, err := coordinator.Build(coordCtx, cfg, instanceID)
	if err != nil {
		coordCancel()
		return nil, err
	}

	// StartHeartbeat 内部立即执行一次 RegisterInstance 并起后台续期 goroutine,
	// 所以这里不需要在外面再单独 RegisterInstance。
	coordinator.StartHeartbeat(coordCtx, coord, cfg.InstanceTTL)

	cooldown.Default().SetCoordinator(coordCtx, coord)
	sticky.Default().SetCoordinator(coordCtx, coord)

	cleanup := func() {
		// 顺序很重要:先 Unregister 让其他实例感知到本实例下线,
		// 然后 cancel ctx 阻止后续 goroutine 续期/订阅,最后 Close。
		unregCtx, unregCancel := context.WithTimeout(context.Background(), 2*time.Second)
		if err := coord.UnregisterInstance(unregCtx); err != nil {
			log.Printf("[Coordinator] Warning: UnregisterInstance failed: %v", err)
		}
		unregCancel()
		coordCancel()
		coordCleanup()
	}

	return &CoordinatorComponents{
		Coordinator: coord,
		Config:      cfg,
		Ctx:         coordCtx,
		Cancel:      coordCancel,
		Cleanup:     cleanup,
	}, nil
}

// AttachCachedReposToCoordinator 把所有 cached repo 接入 coordinator:
//   - SetCoordinator: 让 mutation 发布失效事件
//   - AttachInvalidation: 启动订阅 goroutine,收到非自身事件时 Load()
//
// 必须在 repos.CachedXxxRepo.Load() 之前调用,这样订阅 goroutine 就位后
// 任何后续 mutation 都会被广播。
func AttachCachedReposToCoordinator(ctx context.Context, coord coordinator.Coordinator, repos *DatabaseRepos) {
	repos.CachedProviderRepo.SetCoordinator(coord)
	repos.CachedRouteRepo.SetCoordinator(coord)
	repos.CachedRetryConfigRepo.SetCoordinator(coord)
	repos.CachedRoutingStrategyRepo.SetCoordinator(coord)
	repos.CachedProjectRepo.SetCoordinator(coord)
	repos.CachedAPITokenRepo.SetCoordinator(coord)
	repos.CachedModelMappingRepo.SetCoordinator(coord)
	repos.CachedSessionRepo.SetCoordinator(coord, time.Hour)

	cached.AttachInvalidation(ctx, coord, cached.InvalidateProvider, func() {
		if err := repos.CachedProviderRepo.Load(); err != nil {
			log.Printf("[Cache] reload providers failed: %v", err)
		}
	})
	cached.AttachInvalidation(ctx, coord, cached.InvalidateRoute, func() {
		if err := repos.CachedRouteRepo.Load(); err != nil {
			log.Printf("[Cache] reload routes failed: %v", err)
		}
	})
	cached.AttachInvalidation(ctx, coord, cached.InvalidateRetryConfig, func() {
		if err := repos.CachedRetryConfigRepo.Load(); err != nil {
			log.Printf("[Cache] reload retry configs failed: %v", err)
		}
	})
	cached.AttachInvalidation(ctx, coord, cached.InvalidateRoutingStrategy, func() {
		if err := repos.CachedRoutingStrategyRepo.Load(); err != nil {
			log.Printf("[Cache] reload routing strategies failed: %v", err)
		}
	})
	cached.AttachInvalidation(ctx, coord, cached.InvalidateProject, func() {
		if err := repos.CachedProjectRepo.Load(); err != nil {
			log.Printf("[Cache] reload projects failed: %v", err)
		}
	})
	cached.AttachInvalidation(ctx, coord, cached.InvalidateAPIToken, func() {
		if err := repos.CachedAPITokenRepo.Load(); err != nil {
			log.Printf("[Cache] reload api tokens failed: %v", err)
		}
	})
	cached.AttachInvalidation(ctx, coord, cached.InvalidateModelMapping, func() {
		if err := repos.CachedModelMappingRepo.Load(); err != nil {
			log.Printf("[Cache] reload model mappings failed: %v", err)
		}
	})
}
