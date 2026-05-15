package multiinstance

import (
	"testing"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
)

// 实例 A 创建了一个 provider,实例 B 通过失效事件感知并 reload 本地缓存。
func TestProviderCreateInvalidatesPeer(t *testing.T) {
	c := newCluster(t)
	a := c.newInstance(t, "inst-A")
	b := c.newInstance(t, "inst-B")

	p := &domain.Provider{
		TenantID: domain.DefaultTenantID,
		Name:     "shared-claude",
		Type:     "claude",
	}
	if err := a.Comp.Provider.Create(p); err != nil {
		t.Fatalf("Create provider on A: %v", err)
	}

	// B 应该通过 cache:invalidate:provider 事件 reload 并看到这个 provider
	if !waitFor(t, time.Second, func() bool {
		list, _ := b.Comp.Provider.List(domain.DefaultTenantID)
		for _, prov := range list {
			if prov.Name == "shared-claude" {
				return true
			}
		}
		return false
	}) {
		t.Fatal("instance B never saw provider created on A")
	}
}

// 实例 A 删除一个 model_mapping,B 的缓存被失效后看不到该 mapping。
func TestModelMappingDeleteInvalidatesPeer(t *testing.T) {
	c := newCluster(t)
	a := c.newInstance(t, "inst-A")
	b := c.newInstance(t, "inst-B")

	mapping := &domain.ModelMapping{
		TenantID:   domain.DefaultTenantID,
		Scope:      domain.ModelMappingScopeGlobal,
		ClientType: domain.ClientTypeClaude,
		Pattern:    "claude-sonnet",
		Target:     "claude-haiku",
	}
	if err := a.Comp.ModelMapping.Create(mapping); err != nil {
		t.Fatalf("Create mapping: %v", err)
	}

	// 等 B 看到 mapping
	if !waitFor(t, time.Second, func() bool {
		list, _ := b.Comp.ModelMapping.List(domain.DefaultTenantID)
		return len(list) == 1
	}) {
		t.Fatal("B did not see initial mapping creation")
	}

	// A 删除
	if err := a.Comp.ModelMapping.Delete(domain.DefaultTenantID, mapping.ID); err != nil {
		t.Fatalf("Delete mapping: %v", err)
	}

	// B 应被失效事件清空
	if !waitFor(t, time.Second, func() bool {
		list, _ := b.Comp.ModelMapping.List(domain.DefaultTenantID)
		return len(list) == 0
	}) {
		t.Fatal("B did not see mapping deletion")
	}
}

// API token 跨实例可见性:多实例后挂在 B 后面的请求要能识别在 A 上创建的 token。
func TestAPITokenCreateInvalidatesPeer(t *testing.T) {
	c := newCluster(t)
	a := c.newInstance(t, "inst-A")
	b := c.newInstance(t, "inst-B")

	tok := &domain.APIToken{
		TenantID: domain.DefaultTenantID,
		Name:     "shared-token",
		Token:    "sk-multi-instance",
	}
	if err := a.Comp.APIToken.Create(tok); err != nil {
		t.Fatalf("Create token: %v", err)
	}

	if !waitFor(t, time.Second, func() bool {
		got, err := b.Comp.APIToken.GetByToken(domain.DefaultTenantID, "sk-multi-instance")
		return err == nil && got != nil && got.Name == "shared-token"
	}) {
		t.Fatal("B could not authenticate with token created on A")
	}
}
