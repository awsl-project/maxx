// Package multiinstance 集成测试:验证两个或更多 maxx 实例在共享 Redis +
// 共享 DB 下的数据一致性与滚动更新行为。
//
// 测试用 miniredis 模拟 Redis,SQLite 共享文件模拟共享 DB。每个测试构造一个
// `cluster`(包含 N 个 `instance`),每个 instance 自己有 cooldown.Manager
// 与 cached repos,但共享同一份底层存储,从而真实复刻多进程部署的语义。
package multiinstance

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/awsl-project/maxx/internal/cooldown"
	"github.com/awsl-project/maxx/internal/coordinator"
	"github.com/awsl-project/maxx/internal/repository/cached"
	"github.com/awsl-project/maxx/internal/repository/sqlite"
)

// instance 是单进程视图。生产中由 cmd/maxx 装配;这里我们只装配集成测试
// 实际涉及的子系统:DB、coordinator、cooldown.Manager、cached repos。
type instance struct {
	ID    string
	Coord coordinator.Coordinator
	DB    *sqlite.DB
	Mgr   *cooldown.Manager
	Comp  *componentSet
	ctx   context.Context
	stop  context.CancelFunc
}

type componentSet struct {
	Provider     *cached.ProviderRepository
	Route        *cached.RouteRepository
	Session      *cached.SessionRepository
	APIToken     *cached.APITokenRepository
	ModelMapping *cached.ModelMappingRepository
	ProxyRequest *sqlite.ProxyRequestRepository
}

// cluster 是 N 实例共享底层存储的封装。
type cluster struct {
	t      testing.TB
	redis  *miniredis.Miniredis
	dbPath string
}

// newCluster 启动 miniredis 并准备一个共享 SQLite 文件。返回的 cluster
// 拥有 Cleanup 责任,t.Cleanup 已经接管 redis 关停 + 临时文件清理。
func newCluster(t testing.TB) *cluster {
	t.Helper()

	mr := miniredis.RunT(t)

	dbPath := filepath.Join(t.TempDir(), "maxx.db")
	c := &cluster{t: t, redis: mr, dbPath: dbPath}
	return c
}

// RedisURL 返回 miniredis 的 go-redis 兼容 URL
func (c *cluster) RedisURL() string {
	return "redis://" + c.redis.Addr() + "/0"
}

// newInstance 装配一个实例。每个实例:
//   - 单独的 cooldown.Manager(包级单例 cooldown.Default() 在测试里不能用,
//     因为多实例共享一个进程时会互相覆盖)
//   - 自己的 coordinator 客户端,连同一个 miniredis
//   - 自己的 sqlite.DB 句柄,连同一个 dbPath(SQLite 文件级共享 + WAL)
//   - 自己的 cached repos
//
// 启动顺序与 cmd/maxx/main.go 一致:repo → coord → register → cooldown
// 接入 → cached 接入 → 初始 sweep。
func (c *cluster) newInstance(t testing.TB, instanceID string) *instance {
	t.Helper()

	db, err := sqlite.NewDB(c.dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	cfg := coordinator.Config{
		Mode:              coordinator.ModeFailFast,
		RedisURL:          c.RedisURL(),
		InstanceTTL:       30 * time.Second,
		HeartbeatInterval: 10 * time.Second,
		ReconnectInterval: 5 * time.Second,
		SweepInterval:     15 * time.Second,
	}
	ctx, cancel := context.WithCancel(context.Background())
	coord, cleanup, err := coordinator.Build(ctx, cfg, instanceID)
	if err != nil {
		cancel()
		t.Fatalf("build coordinator: %v", err)
	}
	if err := coord.RegisterInstance(ctx, cfg.InstanceTTL); err != nil {
		cleanup()
		cancel()
		t.Fatalf("register instance: %v", err)
	}

	mgr := cooldown.NewManager()
	mgr.SetRepository(sqlite.NewCooldownRepository(db))
	mgr.SetFailureCountRepository(sqlite.NewFailureCountRepository(db))
	if err := mgr.LoadFromDatabase(); err != nil {
		t.Fatalf("load cooldown: %v", err)
	}
	mgr.SetCoordinator(ctx, coord)

	comp := &componentSet{
		Provider:     cached.NewProviderRepository(sqlite.NewProviderRepository(db)),
		Route:        cached.NewRouteRepository(sqlite.NewRouteRepository(db)),
		Session:      cached.NewSessionRepository(sqlite.NewSessionRepository(db)),
		APIToken:     cached.NewAPITokenRepository(sqlite.NewAPITokenRepository(db)),
		ModelMapping: cached.NewModelMappingRepository(sqlite.NewModelMappingRepository(db)),
		ProxyRequest: sqlite.NewProxyRequestRepository(db),
	}

	comp.Provider.SetCoordinator(coord)
	comp.Route.SetCoordinator(coord)
	comp.Session.SetCoordinator(coord, time.Hour)
	comp.APIToken.SetCoordinator(coord)
	comp.ModelMapping.SetCoordinator(coord)

	// 订阅失效事件
	cached.AttachInvalidation(ctx, coord, cached.InvalidateProvider, func() { _ = comp.Provider.Load() })
	cached.AttachInvalidation(ctx, coord, cached.InvalidateRoute, func() { _ = comp.Route.Load() })
	cached.AttachInvalidation(ctx, coord, cached.InvalidateAPIToken, func() { _ = comp.APIToken.Load() })
	cached.AttachInvalidation(ctx, coord, cached.InvalidateModelMapping, func() { _ = comp.ModelMapping.Load() })

	// 预加载缓存
	_ = comp.Provider.Load()
	_ = comp.Route.Load()
	_ = comp.APIToken.Load()
	_ = comp.ModelMapping.Load()

	inst := &instance{
		ID:    instanceID,
		Coord: coord,
		DB:    db,
		Mgr:   mgr,
		Comp:  comp,
		ctx:   ctx,
		stop:  cancel,
	}

	t.Cleanup(func() {
		// 模拟优雅退出:UnregisterInstance → cancel ctx → close coord → close db
		_ = coord.UnregisterInstance(context.Background())
		cancel()
		cleanup()
		_ = db.Close()
	})

	return inst
}

// shutdown 模拟实例优雅下线(在 cluster 销毁前主动停止某个实例,用于
// 滚动更新场景)。t.Cleanup 已经处理掉的实例再调用一次也是无害的。
func (inst *instance) shutdown() {
	_ = inst.Coord.UnregisterInstance(context.Background())
	inst.stop()
}

// kill 模拟实例硬崩溃(不 unregister,仅断 ctx)。用于测试 heartbeat
// 过期后其他实例如何接管该实例的孤儿请求。
func (inst *instance) kill() {
	inst.stop()
}
