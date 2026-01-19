import { useMemo, useRef, useState } from 'react';
import { Plus, Layers, Download, Upload, Search } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { useProviders, useAllProviderStats } from '@/hooks/queries';
import { useStreamingRequests } from '@/hooks/use-streaming';
import type { Provider, ImportResult } from '@/lib/transport';
import { getTransport } from '@/lib/transport';
import { ProviderRow } from './components/provider-row';
import { useQueryClient } from '@tanstack/react-query';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { PageHeader } from '@/components/layout/page-header';
import { PROVIDER_TYPE_CONFIGS, type ProviderTypeKey } from './types';
import { AntigravityQuotasProvider } from '@/contexts/antigravity-quotas-context';

export function ProvidersPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { data: providers, isLoading } = useProviders();
  const { data: providerStats = {} } = useAllProviderStats();
  const { countsByProvider } = useStreamingRequests();
  const [importStatus, setImportStatus] = useState<ImportResult | null>(null);
  const [searchQuery, setSearchQuery] = useState('');
  const fileInputRef = useRef<HTMLInputElement>(null);
  const queryClient = useQueryClient();

  const groupedProviders = useMemo(() => {
    // 按类型分组，使用配置系统中定义的类型
    const groups: Record<ProviderTypeKey, Provider[]> = {
      antigravity: [],
      kiro: [],
      custom: [],
    };

    // Filter providers by search query
    const filteredProviders = providers?.filter((p) => {
      if (!searchQuery.trim()) return true;
      const query = searchQuery.toLowerCase();
      const config = PROVIDER_TYPE_CONFIGS[p.type as ProviderTypeKey];
      const displayInfo = config?.getDisplayInfo(p) || '';
      return p.name.toLowerCase().includes(query) || displayInfo.toLowerCase().includes(query);
    });

    filteredProviders?.forEach((p) => {
      const type = p.type as ProviderTypeKey;
      if (groups[type]) {
        groups[type].push(p);
      } else {
        // 未知类型归入 custom
        groups.custom.push(p);
      }
    });

    // 按 id 倒序排列（新创建的在前）
    for (const key of Object.keys(groups) as ProviderTypeKey[]) {
      groups[key].sort((a, b) => b.id - a.id);
    }

    return groups;
  }, [providers, searchQuery]);

  // Export providers as JSON file
  const handleExport = async () => {
    try {
      const transport = getTransport();
      const data = await transport.exportProviders();
      const blob = new Blob([JSON.stringify(data, null, 2)], {
        type: 'application/json',
      });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = `providers-${new Date().toISOString().split('T')[0]}.json`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(url);
    } catch (error) {
      console.error('Export failed:', error);
    }
  };

  // Import providers from JSON file
  const handleImport = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    if (!file) return;

    try {
      const text = await file.text();
      const data = JSON.parse(text) as Provider[];
      const transport = getTransport();
      const result = await transport.importProviders(data);
      setImportStatus(result);
      queryClient.invalidateQueries({ queryKey: ['providers'] });
      queryClient.invalidateQueries({ queryKey: ['routes'] });
      // Clear file input
      if (fileInputRef.current) {
        fileInputRef.current.value = '';
      }
      // Auto-hide status after 5 seconds
      setTimeout(() => setImportStatus(null), 5000);
    } catch (error) {
      console.error('Import failed:', error);
      setImportStatus({
        imported: 0,
        skipped: 0,
        errors: [`Import failed: ${error}`],
      });
      setTimeout(() => setImportStatus(null), 5000);
    }
  };

  // Provider list
  return (
    <div className="flex flex-col h-full bg-background">
      <PageHeader
        icon={Layers}
        iconClassName="text-blue-500"
        title={t('providers.title')}
        description={t('providers.description', {
          count: providers?.length || 0,
        })}
      >
        <div className="relative">
          <Search
            size={14}
            className="absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground"
          />
          <Input
            placeholder={t('common.search')}
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="pl-9 w-48"
          />
        </div>
        <input
          type="file"
          ref={fileInputRef}
          onChange={handleImport}
          accept=".json"
          className="hidden"
        />
        <Button
          onClick={() => fileInputRef.current?.click()}
          className="flex items-center gap-2"
          title={t('providers.importProviders')}
          variant={'outline'}
        >
          <Upload size={14} />
          <span>{t('common.import')}</span>
        </Button>
        <Button
          onClick={handleExport}
          className="flex items-center gap-2"
          disabled={!providers?.length}
          title={t('providers.exportProviders')}
          variant={'outline'}
        >
          <Download size={14} />
          <span>{t('common.export')}</span>
        </Button>
        <Button onClick={() => navigate('/providers/create')}>
          <Plus size={14} />
          <span>{t('providers.addProvider')}</span>
        </Button>
      </PageHeader>

      <div className="flex-1 overflow-y-auto p-6">
        <div className="mx-auto max-w-7xl">
          {isLoading ? (
            <div className="flex items-center justify-center h-full">
              <div className="text-text-muted">{t('common.loading')}</div>
            </div>
          ) : providers?.length === 0 ? (
            <div className="flex flex-col items-center justify-center h-full text-muted-foreground">
              <Layers size={48} className="mb-4 opacity-50" />
              <p className="text-body">{t('providers.noProviders')}</p>
              <p className="text-caption mt-2">{t('providers.noProvidersHint')}</p>
              <Button
                onClick={() => navigate('/providers/create')}
                className=" mt-6 flex items-center gap-2"
              >
                <Plus size={14} />
                <span>{t('providers.addProvider')}</span>
              </Button>
            </div>
          ) : (
            <AntigravityQuotasProvider>
              <div className="space-y-8">
                {/* 动态渲染各类型分组 */}
                {(Object.keys(PROVIDER_TYPE_CONFIGS) as ProviderTypeKey[]).map((typeKey) => {
                  const typeProviders = groupedProviders[typeKey];
                  if (typeProviders.length === 0) return null;

                  const config = PROVIDER_TYPE_CONFIGS[typeKey];
                  const TypeIcon = config.icon;

                  return (
                    <section key={typeKey} className="space-y-3">
                      <div className="flex items-center gap-2 px-1">
                        <TypeIcon size={16} style={{ color: config.color }} />
                        <h3 className="text-sm font-semibold text-muted-foreground uppercase tracking-wider">
                          {config.label}
                        </h3>
                        <div className="h-px flex-1 bg-border/50 ml-2" />
                      </div>
                      <div className="space-y-3">
                        {typeProviders.map((provider) => (
                          <ProviderRow
                            key={provider.id}
                            provider={provider}
                            stats={providerStats[provider.id]}
                            streamingCount={countsByProvider.get(provider.id) || 0}
                            onClick={() => navigate(`/providers/${provider.id}/edit`)}
                          />
                        ))}
                      </div>
                    </section>
                  );
                })}
              </div>
            </AntigravityQuotasProvider>
          )}
        </div>
      </div>

      {/* Import Status Toast */}
      {importStatus && (
        <div className="fixed bottom-6 right-6 bg-card border border-border rounded-lg shadow-lg p-4">
          <div className="space-y-2">
            <div className="text-sm font-medium text-text-primary">
              {t('providers.importCompleted', {
                imported: importStatus.imported,
                skipped: importStatus.skipped,
              })}
            </div>
            {importStatus.errors.length > 0 && (
              <div className="text-xs text-red-400 space-y-1">
                {importStatus.errors.map((error, i) => (
                  <div key={i}>• {error}</div>
                ))}
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
