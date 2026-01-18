import { useState } from 'react';
import { Radio, Check, Copy } from 'lucide-react';
import { useProxyStatus } from '@/hooks/queries';
import { useSidebar } from '@/components/ui/sidebar';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { useTranslation } from 'react-i18next';

export function NavProxyStatus() {
  const { t } = useTranslation();
  const { data: proxyStatus } = useProxyStatus();
  const { state } = useSidebar();
  const [copied, setCopied] = useState(false);

  const proxyAddress = proxyStatus?.address ?? '...';
  const fullUrl = `http://${proxyAddress}`;
  const isCollapsed = state === 'collapsed';

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(fullUrl);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch (err) {
      console.error('Failed to copy:', err);
    }
  };

  if (isCollapsed) {
    return (
      <Tooltip>
        <TooltipTrigger
          onClick={handleCopy}
          className="relative flex items-center justify-center w-8 h-8 mt-1 rounded-lg bg-emerald-400/10 hover:bg-emerald-400/20 transition-all cursor-pointer group"
        >
          <Radio
            size={16}
            className={`text-emerald-400 transition-all ${
              copied ? 'scale-0 opacity-0' : 'scale-100 opacity-100'
            }`}
          />
          <Check
            size={16}
            className={`absolute text-emerald-400 transition-all ${
              copied ? 'scale-100 opacity-100' : 'scale-0 opacity-0'
            }`}
          />
        </TooltipTrigger>
        <TooltipContent side="right" align="center">
          <div className="flex flex-col gap-1">
            <span className="text-xs text-text-muted">{t('proxy.listeningOn')}</span>
            <span className="font-mono font-medium">{proxyAddress}</span>
            <span className="text-xs text-emerald-400">
              {copied ? t('proxy.copied') : t('proxy.clickToCopy')}
            </span>
          </div>
        </TooltipContent>
      </Tooltip>
    );
  }

  return (
    <div className="h-auto border-none p-2 flex items-center gap-2 w-full rounded-lg transition-all group">
      <div className="w-8 h-8 rounded-lg bg-emerald-400/10 flex items-center justify-center shrink-0 transition-colors cursor-default">
        <Radio size={16} className="text-emerald-400" />
      </div>
      <div className="flex flex-col items-start flex-1 min-w-0">
        <span className="text-caption text-text-muted">{t('proxy.listeningOn')}</span>
        <span className="font-mono font-medium text-text-primary truncate">{proxyAddress}</span>
      </div>
      <button
        type="button"
        onClick={handleCopy}
        className="shrink-0 text-muted-foreground relative w-4 h-4 cursor-pointer hover:text-foreground transition-colors"
        title={`Click to copy: ${fullUrl}`}
      >
        <Copy
          size={14}
          className={`absolute inset-0 transition-all ${
            copied ? 'scale-0 opacity-0' : 'scale-100 opacity-0 group-hover:opacity-100'
          }`}
        />
        <Check
          size={18}
          className={`absolute inset-0 text-emerald-400 transition-all ${
            copied ? 'scale-100 opacity-100' : 'scale-0 opacity-0'
          }`}
        />
      </button>
    </div>
  );
}
