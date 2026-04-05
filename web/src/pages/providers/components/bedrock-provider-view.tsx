import { useState } from 'react';
import {
  Cloud,
  ChevronLeft,
  Trash2,
  Key,
  Globe,
  Eye,
  EyeOff,
  Copy,
  Check,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import type { Provider } from '@/lib/transport';
import { useUpdateProvider } from '@/hooks/queries';
import { Button, Switch } from '@/components/ui';
import { PageHeader } from '@/components/layout/page-header';
import { BEDROCK_COLOR } from '../types';
import { ProviderProxyURLCard } from './provider-proxy-url-card';

interface BedrockProviderViewProps {
  provider: Provider;
  onDelete: () => void;
  onClose: () => void;
}

export function BedrockProviderView({ provider, onDelete, onClose }: BedrockProviderViewProps) {
  const { t } = useTranslation();
  const updateProvider = useUpdateProvider();
  const config = provider.config?.bedrock;

  const [showSecret, setShowSecret] = useState(false);
  const [copied, setCopied] = useState<string | null>(null);

  const handleCopy = (text: string, key: string) => {
    navigator.clipboard.writeText(text);
    setCopied(key);
    setTimeout(() => setCopied(null), 2000);
  };

  const handleToggleCooldown = async (checked: boolean) => {
    await updateProvider.mutateAsync({
      id: provider.id,
      data: {
        config: {
          ...provider.config,
          disableErrorCooldown: checked,
        },
      },
    });
  };

  return (
    <div className="flex flex-col h-full">
      <PageHeader
        icon={<ChevronLeft className="cursor-pointer" onClick={onClose} />}
        title={provider.name}
        description={`AWS Bedrock (${config?.region || 'us-east-1'})`}
      >
        <Button onClick={onDelete} variant="destructive">
          <Trash2 size={14} />
          {t('provider.delete')}
        </Button>
      </PageHeader>

      <div className="flex-1 overflow-y-auto p-6">
        <div className="mx-auto max-w-7xl space-y-8">
          {/* Provider Info Card */}
          <div
            className="rounded-xl border p-6 space-y-4"
            style={{ borderColor: `color-mix(in oklch, ${BEDROCK_COLOR} 30%, transparent)` }}
          >
            <div className="flex items-center gap-3">
              <div
                className="size-12 rounded-lg flex items-center justify-center"
                style={{ backgroundColor: `color-mix(in oklch, ${BEDROCK_COLOR} 15%, transparent)` }}
              >
                <Cloud className="size-6" style={{ color: BEDROCK_COLOR }} />
              </div>
              <div>
                <h3 className="text-lg font-semibold">{provider.name}</h3>
                <p className="text-sm text-muted-foreground">AWS Bedrock</p>
              </div>
            </div>

            <div className="grid gap-4">
              {/* Region */}
              <div className="flex items-center justify-between p-3 bg-muted/50 rounded-lg">
                <div className="flex items-center gap-2 text-sm">
                  <Globe size={14} className="text-muted-foreground" />
                  <span className="text-muted-foreground">Region</span>
                </div>
                <span className="text-sm font-mono">{config?.region || 'us-east-1'}</span>
              </div>

              {/* Model Prefix */}
              <div className="flex items-center justify-between p-3 bg-muted/50 rounded-lg">
                <div className="flex items-center gap-2 text-sm">
                  <span className="text-muted-foreground">Model Prefix</span>
                </div>
                <span className="text-sm font-mono">{config?.modelPrefix || 'us'}</span>
              </div>

              {/* Access Key ID */}
              <div className="flex items-center justify-between p-3 bg-muted/50 rounded-lg">
                <div className="flex items-center gap-2 text-sm">
                  <Key size={14} className="text-muted-foreground" />
                  <span className="text-muted-foreground">Access Key ID</span>
                </div>
                <div className="flex items-center gap-2">
                  <span className="text-sm font-mono">
                    {config?.accessKeyId
                      ? config.accessKeyId.slice(0, 8) + '...' + config.accessKeyId.slice(-4)
                      : '-'}
                  </span>
                  {config?.accessKeyId && (
                    <button
                      onClick={() => handleCopy(config.accessKeyId, 'akid')}
                      className="text-muted-foreground hover:text-foreground transition-colors"
                    >
                      {copied === 'akid' ? (
                        <Check className="h-3.5 w-3.5" />
                      ) : (
                        <Copy className="h-3.5 w-3.5" />
                      )}
                    </button>
                  )}
                </div>
              </div>

              {/* Secret Access Key */}
              <div className="flex items-center justify-between p-3 bg-muted/50 rounded-lg">
                <div className="flex items-center gap-2 text-sm">
                  <Key size={14} className="text-muted-foreground" />
                  <span className="text-muted-foreground">Secret Access Key</span>
                </div>
                <div className="flex items-center gap-2">
                  <span className="text-sm font-mono">
                    {showSecret
                      ? config?.secretAccessKey || '-'
                      : config?.secretAccessKey
                        ? '***' + config.secretAccessKey.slice(-4)
                        : '-'}
                  </span>
                  <button
                    onClick={() => setShowSecret(!showSecret)}
                    className="text-muted-foreground hover:text-foreground transition-colors"
                  >
                    {showSecret ? <EyeOff className="h-3.5 w-3.5" /> : <Eye className="h-3.5 w-3.5" />}
                  </button>
                  {config?.secretAccessKey && (
                    <button
                      onClick={() => handleCopy(config.secretAccessKey, 'sak')}
                      className="text-muted-foreground hover:text-foreground transition-colors"
                    >
                      {copied === 'sak' ? (
                        <Check className="h-3.5 w-3.5" />
                      ) : (
                        <Copy className="h-3.5 w-3.5" />
                      )}
                    </button>
                  )}
                </div>
              </div>
            </div>
          </div>

          {/* Proxy URL Card */}
          <ProviderProxyURLCard provider={provider} />

          {/* Error Cooldown */}
          <div className="space-y-4">
            <h3 className="text-lg font-semibold text-text-primary border-b border-border pb-2">
              {t('provider.errorCooldownTitle')}
            </h3>
            <div className="flex items-center justify-between p-4 bg-card border border-border rounded-xl">
              <div className="pr-4">
                <div className="text-sm font-medium text-foreground">
                  {t('provider.disableErrorCooldown')}
                </div>
                <p className="text-xs text-muted-foreground mt-1">
                  {t('provider.disableErrorCooldownDesc')}
                </p>
              </div>
              <Switch
                checked={!!provider.config?.disableErrorCooldown}
                onCheckedChange={handleToggleCooldown}
              />
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
