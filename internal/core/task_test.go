package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/coordinator"
	"github.com/awsl-project/maxx/internal/domain"
	"github.com/awsl-project/maxx/internal/repository"
	"github.com/awsl-project/maxx/internal/repository/sqlite"
)

type fakeSessionRepo struct {
	deleteCalls int
	lastBefore  time.Time
}

func (f *fakeSessionRepo) Create(session *domain.Session) error {
	return nil
}

func (f *fakeSessionRepo) Update(session *domain.Session) error {
	return nil
}

func (f *fakeSessionRepo) Touch(tenantID uint64, sessionID string, touchedAt time.Time) error {
	return nil
}

func (f *fakeSessionRepo) GetBySessionID(tenantID uint64, sessionID string) (*domain.Session, error) {
	return nil, domain.ErrNotFound
}

func (f *fakeSessionRepo) List(tenantID uint64) ([]*domain.Session, error) {
	return nil, nil
}

func (f *fakeSessionRepo) ListExpiredKeys(before time.Time) ([]repository.SessionKey, error) {
	return nil, nil
}

func (f *fakeSessionRepo) DeleteOlderThan(before time.Time) (int64, error) {
	f.deleteCalls++
	f.lastBefore = before
	return 0, nil
}

type fakeSettingRepo struct {
	values map[string]string
}

func (f *fakeSettingRepo) Get(key string) (string, error) {
	if f.values == nil {
		return "", nil
	}
	return f.values[key], nil
}

func (f *fakeSettingRepo) Set(key, value string) error {
	if f.values == nil {
		f.values = make(map[string]string)
	}
	f.values[key] = value
	return nil
}

func (f *fakeSettingRepo) GetAll() ([]*domain.SystemSetting, error) {
	return nil, nil
}

func (f *fakeSettingRepo) Delete(key string) error {
	if f.values != nil {
		delete(f.values, key)
	}
	return nil
}

func TestCleanupOldSessionsUsesDefaultRetention(t *testing.T) {
	sessionRepo := &fakeSessionRepo{}
	deps := BackgroundTaskDeps{
		SessionRepo: sessionRepo,
		Settings:    &fakeSettingRepo{},
	}

	start := time.Now()
	deps.cleanupOldSessions()
	end := time.Now()

	if sessionRepo.deleteCalls != 1 {
		t.Fatalf("Expected cleanup to run once, got %d", sessionRepo.deleteCalls)
	}

	expectedMin := start.Add(-defaultSessionRetentionHours * time.Hour).Add(-2 * time.Second)
	expectedMax := end.Add(-defaultSessionRetentionHours * time.Hour).Add(2 * time.Second)
	if sessionRepo.lastBefore.Before(expectedMin) || sessionRepo.lastBefore.After(expectedMax) {
		t.Fatalf("Expected cleanup cutoff near %v..%v, got %v", expectedMin, expectedMax, sessionRepo.lastBefore)
	}
}

// seedRequest creates a ProxyRequest with detail and backdates created_at to oldTime
func seedRequest(t *testing.T, db *sqlite.DB, repo *sqlite.ProxyRequestRepository, status string, oldTime time.Time, idx int) *domain.ProxyRequest {
	t.Helper()
	r := &domain.ProxyRequest{
		TenantID:     1,
		InstanceID:   "i",
		RequestID:    "r" + status,
		ClientType:   domain.ClientType("claude"),
		RequestModel: "m",
		StartTime:    oldTime,
		Status:       status,
		StatusCode:   200,
		RequestInfo:  &domain.RequestInfo{Method: "POST", URL: "u", Body: "b"},
		ResponseInfo: &domain.ResponseInfo{Status: 200, Body: "r"},
	}
	r.RequestID = r.RequestID + "-" + time.Now().Format("150405.000000000")
	if err := repo.Create(r); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := db.GormDB().Table("proxy_requests").Where("id = ?", r.ID).
		Update("created_at", oldTime.UnixMilli()).Error; err != nil {
		t.Fatalf("backdate: %v", err)
	}
	return r
}

func reloadDetailEmpty(t *testing.T, db *sqlite.DB, id uint64) bool {
	t.Helper()
	var row struct {
		RequestInfo  []byte
		ResponseInfo []byte
	}
	if err := db.GormDB().Table("proxy_requests").
		Select("request_info, response_info").
		Where("id = ?", id).Scan(&row).Error; err != nil {
		t.Fatalf("scan: %v", err)
	}
	return len(row.RequestInfo) == 0 && len(row.ResponseInfo) == 0
}

func TestCleanupOldRequestDetails_SplitMode(t *testing.T) {
	now := time.Now()
	oldTime := now.Add(-2 * time.Hour) // 老到一定被清的时间

	t.Run("split=false uses unified key, clears all statuses", func(t *testing.T) {
		db, err := sqlite.NewDBWithDSN("sqlite://:memory:")
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		defer db.Close()
		reqRepo := sqlite.NewProxyRequestRepository(db)
		settings := &fakeSettingRepo{values: map[string]string{
			domain.SettingKeyRequestDetailRetentionSeconds: "60", // 1 分钟，oldTime 已过期
		}}
		deps := BackgroundTaskDeps{
			ProxyRequest: reqRepo,
			Settings:     settings,
		}

		ok := seedRequest(t, db, reqRepo, "COMPLETED", oldTime, 1)
		bad := seedRequest(t, db, reqRepo, "FAILED", oldTime, 2)

		deps.cleanupOldRequestDetails()

		if !reloadDetailEmpty(t, db, ok.ID) {
			t.Error("split=false should clear COMPLETED")
		}
		if !reloadDetailEmpty(t, db, bad.ID) {
			t.Error("split=false should clear FAILED too")
		}
	})

	t.Run("split=true with success=60 failed=-1 only clears success", func(t *testing.T) {
		db, err := sqlite.NewDBWithDSN("sqlite://:memory:")
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		defer db.Close()
		reqRepo := sqlite.NewProxyRequestRepository(db)
		settings := &fakeSettingRepo{values: map[string]string{
			domain.SettingKeyRequestDetailRetentionSplitEnabled:   "true",
			domain.SettingKeyRequestDetailRetentionSecondsSuccess: "60",
			domain.SettingKeyRequestDetailRetentionSecondsFailed:  "-1",
		}}
		deps := BackgroundTaskDeps{
			ProxyRequest: reqRepo,
			Settings:     settings,
		}

		ok := seedRequest(t, db, reqRepo, "COMPLETED", oldTime, 1)
		bad := seedRequest(t, db, reqRepo, "FAILED", oldTime, 2)
		can := seedRequest(t, db, reqRepo, "CANCELLED", oldTime, 3)

		deps.cleanupOldRequestDetails()

		if !reloadDetailEmpty(t, db, ok.ID) {
			t.Error("COMPLETED should be cleared by success retention")
		}
		if reloadDetailEmpty(t, db, bad.ID) {
			t.Error("FAILED should be retained when failed=-1 (forever)")
		}
		if reloadDetailEmpty(t, db, can.ID) {
			t.Error("CANCELLED should be retained when failed=-1 (forever)")
		}
	})

	t.Run("split=true success unset falls back to unified", func(t *testing.T) {
		db, err := sqlite.NewDBWithDSN("sqlite://:memory:")
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		defer db.Close()
		reqRepo := sqlite.NewProxyRequestRepository(db)
		settings := &fakeSettingRepo{values: map[string]string{
			domain.SettingKeyRequestDetailRetentionSeconds:       "60",   // 统一 key 作回退
			domain.SettingKeyRequestDetailRetentionSplitEnabled:  "true", // split 开启但 _success/_failed 未设置
			domain.SettingKeyRequestDetailRetentionSecondsFailed: "-1",
		}}
		deps := BackgroundTaskDeps{
			ProxyRequest: reqRepo,
			Settings:     settings,
		}

		ok := seedRequest(t, db, reqRepo, "COMPLETED", oldTime, 1)
		bad := seedRequest(t, db, reqRepo, "FAILED", oldTime, 2)

		deps.cleanupOldRequestDetails()

		if !reloadDetailEmpty(t, db, ok.ID) {
			t.Error("success should fall back to unified=60 and clear")
		}
		if reloadDetailEmpty(t, db, bad.ID) {
			t.Error("failed=-1 explicit, should retain")
		}
	})

	t.Run("split=true success=0 aggressively clears completed without affecting failed", func(t *testing.T) {
		// 回归 Codex P1：当 success=0、failed != 0 时，ingress 不再立即清，
		// 必须由后台 cleanup 用 cutoff=now 清掉 success 行；failed 行保留
		db, err := sqlite.NewDBWithDSN("sqlite://:memory:")
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		defer db.Close()
		reqRepo := sqlite.NewProxyRequestRepository(db)
		settings := &fakeSettingRepo{values: map[string]string{
			domain.SettingKeyRequestDetailRetentionSplitEnabled:   "true",
			domain.SettingKeyRequestDetailRetentionSecondsSuccess: "0",
			domain.SettingKeyRequestDetailRetentionSecondsFailed:  "-1",
		}}
		deps := BackgroundTaskDeps{
			ProxyRequest: reqRepo,
			Settings:     settings,
		}

		// 故意用"刚刚创建"的时间戳——success=0 也应该把它清掉
		freshOK := seedRequest(t, db, reqRepo, "COMPLETED", time.Now().Add(-100*time.Millisecond), 1)
		freshBad := seedRequest(t, db, reqRepo, "FAILED", time.Now().Add(-100*time.Millisecond), 2)

		deps.cleanupOldRequestDetails()

		if !reloadDetailEmpty(t, db, freshOK.ID) {
			t.Error("success=0 must clear COMPLETED even when fresh (cutoff=now)")
		}
		if reloadDetailEmpty(t, db, freshBad.ID) {
			t.Error("failed=-1 must retain")
		}
	})

	t.Run("split=true protects in-flight PENDING/IN_PROGRESS from failed bucket", func(t *testing.T) {
		// 回归 Codex r2 P1：长流式/排队请求的 status 是 PENDING/IN_PROGRESS，
		// 即便已超过 failed 保留时间也不能被清空 body（仍在写入中）
		db, err := sqlite.NewDBWithDSN("sqlite://:memory:")
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		defer db.Close()
		reqRepo := sqlite.NewProxyRequestRepository(db)
		settings := &fakeSettingRepo{values: map[string]string{
			domain.SettingKeyRequestDetailRetentionSplitEnabled:   "true",
			domain.SettingKeyRequestDetailRetentionSecondsSuccess: "-1",
			domain.SettingKeyRequestDetailRetentionSecondsFailed:  "60",
		}}
		deps := BackgroundTaskDeps{
			ProxyRequest: reqRepo,
			Settings:     settings,
		}

		// 故意把 created_at 拉得很老，模拟一个跑了很久还没结束的流式请求
		stalePending := seedRequest(t, db, reqRepo, "PENDING", oldTime, 1)
		staleInProg := seedRequest(t, db, reqRepo, "IN_PROGRESS", oldTime, 2)

		deps.cleanupOldRequestDetails()

		if reloadDetailEmpty(t, db, stalePending.ID) {
			t.Error("PENDING must not be cleaned (still in-flight)")
		}
		if reloadDetailEmpty(t, db, staleInProg.ID) {
			t.Error("IN_PROGRESS must not be cleaned (still in-flight)")
		}
	})

	t.Run("all-unset defaults: split on, success 1d cleared, failed 3d retained", func(t *testing.T) {
		// 事故修复回归：历史默认 -1（永久）撑爆磁盘。现默认 split 开启，
		// 成功详情保留 1 天（86400s）、失败详情保留 3 天（259200s）。
		// oldTime = now-2h：已过成功 1 天窗口？否——2h < 1d，成功应保留。
		// 用一个超过 1 天但不足 3 天的时间戳来验证成功被清、失败仍保留。
		db, err := sqlite.NewDBWithDSN("sqlite://:memory:")
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		defer db.Close()
		reqRepo := sqlite.NewProxyRequestRepository(db)
		settings := &fakeSettingRepo{} // 全部默认
		deps := BackgroundTaskDeps{
			ProxyRequest: reqRepo,
			Settings:     settings,
		}

		// 校验解析出来的默认值确实是有限的、分桶的
		successSec, failedSec, split := deps.requestDetailRetentionConfig()
		if !split {
			t.Fatalf("default should enable split, got split=false")
		}
		if successSec != domain.DefaultRequestDetailRetentionSecondsSuccess {
			t.Fatalf("default successSec = %d, want %d", successSec, domain.DefaultRequestDetailRetentionSecondsSuccess)
		}
		if failedSec != domain.DefaultRequestDetailRetentionSecondsFailed {
			t.Fatalf("default failedSec = %d, want %d", failedSec, domain.DefaultRequestDetailRetentionSecondsFailed)
		}

		betweenOneAndThreeDays := now.Add(-48 * time.Hour) // 2 天：过成功 1 天窗口，未过失败 3 天窗口
		okOld := seedRequest(t, db, reqRepo, "COMPLETED", betweenOneAndThreeDays, 1)
		badOld := seedRequest(t, db, reqRepo, "FAILED", betweenOneAndThreeDays, 2)
		// 很新的请求：两桶都应保留
		okFresh := seedRequest(t, db, reqRepo, "COMPLETED", now.Add(-1*time.Minute), 3)

		deps.cleanupOldRequestDetails()

		if !reloadDetailEmpty(t, db, okOld.ID) {
			t.Error("2-day-old COMPLETED should be cleared by default success=86400 (1d)")
		}
		if reloadDetailEmpty(t, db, badOld.ID) {
			t.Error("2-day-old FAILED should be retained by default failed=259200 (3d)")
		}
		if reloadDetailEmpty(t, db, okFresh.ID) {
			t.Error("fresh COMPLETED should be retained (within 1d window)")
		}
	})

	t.Run("explicit unified -1 with split=false still means forever (opt-in preserved)", func(t *testing.T) {
		// 运营者仍可显式选择永久保存：split=false + unified=-1
		db, err := sqlite.NewDBWithDSN("sqlite://:memory:")
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		defer db.Close()
		reqRepo := sqlite.NewProxyRequestRepository(db)
		settings := &fakeSettingRepo{values: map[string]string{
			domain.SettingKeyRequestDetailRetentionSplitEnabled: "false",
			domain.SettingKeyRequestDetailRetentionSeconds:      "-1",
		}}
		deps := BackgroundTaskDeps{
			ProxyRequest: reqRepo,
			Settings:     settings,
		}

		ok := seedRequest(t, db, reqRepo, "COMPLETED", oldTime, 1)
		bad := seedRequest(t, db, reqRepo, "FAILED", oldTime, 2)

		deps.cleanupOldRequestDetails()

		if reloadDetailEmpty(t, db, ok.ID) {
			t.Error("explicit -1 should retain COMPLETED forever")
		}
		if reloadDetailEmpty(t, db, bad.ID) {
			t.Error("explicit -1 should retain FAILED forever")
		}
	})
}

// fakeCoordinator implements coordinator.Coordinator just enough for isCleanupLeader tests.
// All non-leader methods return zero values; only InstanceID + ListAliveInstances behavior matters here.
type fakeCoordinator struct {
	id    string
	alive []string
	err   error
}

func (f *fakeCoordinator) InstanceID() string                            { return f.id }
func (f *fakeCoordinator) Publish(context.Context, string, []byte) error { return nil }
func (f *fakeCoordinator) Subscribe(context.Context, string) (<-chan coordinator.Message, error) {
	return nil, nil
}
func (f *fakeCoordinator) Get(context.Context, string) ([]byte, error)              { return nil, nil }
func (f *fakeCoordinator) Set(context.Context, string, []byte, time.Duration) error { return nil }
func (f *fakeCoordinator) Del(context.Context, string) error                        { return nil }
func (f *fakeCoordinator) RegisterInstance(context.Context, time.Duration) error    { return nil }
func (f *fakeCoordinator) RefreshInstance(context.Context, time.Duration) error     { return nil }
func (f *fakeCoordinator) UnregisterInstance(context.Context) error                 { return nil }
func (f *fakeCoordinator) ListAliveInstances(context.Context) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	// 返回新切片,模拟真实实现(memory/redis 都是新切片)。防御性 copy 让 isCleanupLeader 的
	// sort.Strings 不会污染 fixture。
	out := make([]string, len(f.alive))
	copy(out, f.alive)
	return out, nil
}
func (f *fakeCoordinator) Close() error { return nil }

func TestIsCleanupLeader(t *testing.T) {
	tests := []struct {
		name    string
		coord   coordinator.Coordinator
		wantLed bool
	}{
		{"nil coordinator → always leader (single-instance fallback)", nil, true},
		{"only self alive", &fakeCoordinator{id: "a", alive: []string{"a"}}, true},
		{"self is smallest of many", &fakeCoordinator{id: "a", alive: []string{"c", "a", "b"}}, true},
		{"self is not smallest", &fakeCoordinator{id: "b", alive: []string{"c", "a", "b"}}, false},
		{"list returns error → conservative non-leader", &fakeCoordinator{id: "a", err: errors.New("boom")}, false},
		{"empty alive list → conservative non-leader", &fakeCoordinator{id: "a", alive: nil}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := &BackgroundTaskDeps{Coordinator: tt.coord}
			if got := deps.isCleanupLeader(); got != tt.wantLed {
				t.Errorf("isCleanupLeader() = %v, want %v", got, tt.wantLed)
			}
		})
	}
}

func TestCheckDetailCleanupIndexHealth_NilDBNoPanic(t *testing.T) {
	// 测试调用方未提供 DB 时(常见于 unit test 场景),health check 必须安全 no-op。
	deps := &BackgroundTaskDeps{}
	deps.checkDetailCleanupIndexHealth() // must not panic
}

func TestCheckDetailCleanupIndexHealth_SQLiteSkipped(t *testing.T) {
	// SQLite 用 partial index,index_name 与 MySQL 不同;health check 必须直接跳过
	// 而不是对着 SQLite 的 sqlite_master 跑 information_schema 查询。
	db, err := sqlite.NewDBWithDSN("sqlite://:memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	deps := &BackgroundTaskDeps{DB: db}
	deps.checkDetailCleanupIndexHealth() // must not error/panic on non-MySQL
}

func TestRunCleanupTasksSkipsWhenNotLeader(t *testing.T) {
	sessionRepo := &fakeSessionRepo{}
	deps := BackgroundTaskDeps{
		SessionRepo: sessionRepo,
		Settings:    &fakeSettingRepo{},
		// id="b" 不是最小,leader 应判为 "a"
		Coordinator: &fakeCoordinator{id: "b", alive: []string{"a", "b"}},
	}
	deps.runCleanupTasks()
	if sessionRepo.deleteCalls != 0 {
		t.Fatalf("non-leader instance should not run cleanup, got %d delete calls", sessionRepo.deleteCalls)
	}
}

func TestCleanupOldSessionsRespectsDisabledSetting(t *testing.T) {
	sessionRepo := &fakeSessionRepo{}
	deps := BackgroundTaskDeps{
		SessionRepo: sessionRepo,
		Settings: &fakeSettingRepo{
			values: map[string]string{
				domain.SettingKeySessionRetentionHours: "0",
			},
		},
	}

	deps.cleanupOldSessions()

	if sessionRepo.deleteCalls != 0 {
		t.Fatalf("Expected cleanup to be disabled, got %d calls", sessionRepo.deleteCalls)
	}
}
