package core

import (
	"testing"
	"time"

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

	t.Run("retention=-1 (default) is no-op", func(t *testing.T) {
		db, err := sqlite.NewDBWithDSN("sqlite://:memory:")
		if err != nil {
			t.Fatalf("open db: %v", err)
		}
		defer db.Close()
		reqRepo := sqlite.NewProxyRequestRepository(db)
		settings := &fakeSettingRepo{} // 全部默认（即 -1）
		deps := BackgroundTaskDeps{
			ProxyRequest: reqRepo,
			Settings:     settings,
		}

		ok := seedRequest(t, db, reqRepo, "COMPLETED", oldTime, 1)
		bad := seedRequest(t, db, reqRepo, "FAILED", oldTime, 2)

		deps.cleanupOldRequestDetails()

		if reloadDetailEmpty(t, db, ok.ID) {
			t.Error("default -1 should retain")
		}
		if reloadDetailEmpty(t, db, bad.ID) {
			t.Error("default -1 should retain")
		}
	})
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
