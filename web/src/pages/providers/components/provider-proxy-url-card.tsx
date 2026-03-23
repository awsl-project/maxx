import { useState } from 'react';
import { Check, Copy, Link as LinkIcon } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import type { Provider } from '@/lib/transport';
import { Button } from '@/components/ui/button';

export function ProviderProxyURLCard({ provider }: { provider: Provider }) {
  const { t } = useTranslation();
  const [copied, setCopied] = useState(false);
  const providerProxyUrl = `${window.location.origin}/provider/${provider.id}/`;
  const copyLabel = copied ? t('proxy.copied') : t('projects.copyUrl');

  const copyToClipboard = async () => {
    try {
      await navigator.clipboard.writeText(providerProxyUrl);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2000);
    } catch (error) {
      console.error('Failed to copy provider proxy URL', error);
      setCopied(false);
    }
  };

  return (
    <div className="rounded-xl border border-border bg-card p-4 md:p-5">
      <div className="flex items-start gap-3">
        <div className="mt-0.5 rounded-lg border border-border bg-muted p-2 text-muted-foreground">
          <LinkIcon size={16} />
        </div>
        <div className="min-w-0 flex-1">
          <div className="mb-1 text-sm font-semibold text-foreground">{t('provider.proxyUrl')}</div>
          <p className="mb-3 text-xs text-muted-foreground">
            {t('projects.proxyConfigDesc')}
          </p>
          <div className="flex items-center gap-2">
            <code
              data-testid="provider-proxy-url"
              className="flex-1 rounded border border-border bg-muted/60 px-3 py-2 text-xs text-foreground font-mono break-all"
            >
              {providerProxyUrl}
            </code>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="h-8 w-8 shrink-0 p-0"
              title={copyLabel}
              aria-label={copyLabel}
              onClick={copyToClipboard}
            >
              {copied ? (
                <Check className="h-4 w-4 text-green-500" />
              ) : (
                <Copy className="h-4 w-4" />
              )}
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}
