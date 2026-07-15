package grok

import (
	"github.com/awsl-project/maxx/internal/adapter/provider"
	cliproxyapi "github.com/awsl-project/maxx/internal/adapter/provider/cliproxyapi_grok"
	"github.com/awsl-project/maxx/internal/domain"
)

func init() {
	provider.RegisterAdapterFactory("grok", NewAdapter)
}

func NewAdapter(p *domain.Provider) (provider.ProviderAdapter, error) {
	return cliproxyapi.NewAdapter(p)
}
