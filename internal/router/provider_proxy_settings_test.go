package router

import (
	"testing"

	"github.com/awsl-project/maxx/internal/domain"
)

type providerProxySettingRepo struct {
	values map[string]string
}

func (r *providerProxySettingRepo) Get(key string) (string, error) {
	return r.values[key], nil
}

func (r *providerProxySettingRepo) Set(key, value string) error {
	if r.values == nil {
		r.values = make(map[string]string)
	}
	r.values[key] = value
	return nil
}

func (r *providerProxySettingRepo) GetAll() ([]*domain.SystemSetting, error) { return nil, nil }

func (r *providerProxySettingRepo) Delete(key string) error {
	delete(r.values, key)
	return nil
}

func TestProviderForAdapterClearsDisabledManagedProxy(t *testing.T) {
	r := &Router{settingRepo: &providerProxySettingRepo{values: map[string]string{
		domain.SettingKeyOutboundProxies: `[{"id":"p1","name":"slow","url":"http://127.0.0.1:7890","disabled":true}]`,
	}}}
	p := &domain.Provider{ID: 1, Type: "custom", Config: &domain.ProviderConfig{ProxyURL: "http://127.0.0.1:7890"}}

	adapterProvider := r.providerForAdapter(p)
	if adapterProvider == p {
		t.Fatal("expected disabled proxy to return a cloned provider for adapter construction")
	}
	if got := adapterProvider.Config.ProxyURL; got != "" {
		t.Fatalf("adapter proxyURL = %q, want empty for disabled managed proxy", got)
	}
	if got := p.Config.ProxyURL; got != "http://127.0.0.1:7890" {
		t.Fatalf("stored provider proxyURL mutated to %q", got)
	}
}

func TestProviderForAdapterKeepsEnabledManagedProxy(t *testing.T) {
	r := &Router{settingRepo: &providerProxySettingRepo{values: map[string]string{
		domain.SettingKeyOutboundProxies: `[{"id":"p1","name":"fast","url":"http://127.0.0.1:7890","disabled":false}]`,
	}}}
	p := &domain.Provider{ID: 1, Type: "custom", Config: &domain.ProviderConfig{ProxyURL: "http://127.0.0.1:7890"}}

	adapterProvider := r.providerForAdapter(p)
	if adapterProvider != p {
		t.Fatal("expected enabled managed proxy to keep original provider")
	}
	if got := adapterProvider.Config.ProxyURL; got != "http://127.0.0.1:7890" {
		t.Fatalf("adapter proxyURL = %q, want original proxy", got)
	}
}
