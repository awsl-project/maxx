import { useState } from 'react';
import { Radio, Check, Copy } from 'lucide-react';
import { useProxyStatus } from '@/hooks/queries';
import { useSidebar } from '@/components/ui/sidebar';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { useTranslation } from 'react-i18next';
import { cn } from '@/lib/utils';

export function NavProxyStatus() {
  const { t } = useTranslation();
  const { data: proxyStatus } = useProxyStatus();
  const { state } = useSidebar();
  const [copied, setCopied] = useState(false);

  const proxyAddress = proxyStatus?.address ?? '...';
  const fullUrl = `http://${proxyAddress}`;
  const isCollapsed = state === 'collapsed';
  const versionDisplay = proxyStatus?.version ?? '...';

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
          className="relative flex items-center justify-center w-8 h-8 mt-1 rounded-xl bg-emerald-500/10 ring-1 ring-emerald-500/20 hover:bg-emerald-500/20 hover:ring-emerald-500/30 transition-all cursor-pointer group"
        >
          <Radio
            size={16}
            className={cn(
              'text-emerald-400 transition-all duration-200',
              copied ? 'scale-0 opacity-0' : 'scale-100 opacity-100',
            )}
          />
          <Check
            size={16}
            className={cn(
              'absolute text-emerald-400 transition-all duration-200',
              copied ? 'scale-100 opacity-100' : 'scale-0 opacity-0',
            )}
          />
        </TooltipTrigger>
        <TooltipContent side="right" align="center" className="p-3">
          <div className="flex flex-col gap-1.5">
            <div className="flex items-center gap-2">
              <span className="text-[10px] font-bold text-emerald-400 uppercase tracking-wider">
                {versionDisplay}
              </span>
              <span className="text-[10px] text-muted-foreground">{t('proxy.listeningOn')}</span>
            </div>
            <span className="font-mono text-sm font-semibold">{proxyAddress}</span>
            <span className="text-xs text-emerald-400/80">
              {copied ? t('proxy.copied') : t('proxy.clickToCopy')}
            </span>
          </div>
        </TooltipContent>
      </Tooltip>
    );
  }

  return (
    <div
      onClick={handleCopy}
      className={cn(
        'relative p-2.5 rounded-xl transition-all duration-200 cursor-pointer group/proxy',
        'bg-emerald-500/5 hover:bg-emerald-500/10',
        'ring-1 ring-emerald-500/10 hover:ring-emerald-500/20',
      )}
    >
      <div className="flex items-center gap-2.5">
        {/* Icon */}
        <div className="relative w-9 h-9 rounded-lg bg-emerald-500/15 flex items-center justify-center shrink-0">
          <Radio size={18} className="text-emerald-400" />
        </div>

        {/* Text Content */}
        <div className="flex-1 min-w-0 flex flex-col gap-0.5">
          {/* Version + Status */}
          <div className="flex items-center gap-1.5">
            <span className="text-[10px] font-bold text-emerald-400 uppercase tracking-wide">
              {versionDisplay}
            </span>
            <span className="text-[10px] text-muted-foreground/70">{t('proxy.listeningOn')}</span>
          </div>
          {/* Address */}
          <span className="font-mono text-[13px] font-medium text-foreground truncate">
            {proxyAddress}
          </span>
        </div>

        {/* Copy Button */}
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation();
            handleCopy();
          }}
          className={cn(
            'shrink-0 relative w-7 h-7 rounded-md flex items-center justify-center',
            'hover:bg-emerald-500/10',
            'transition-all duration-200',
          )}
          title={`Click to copy: ${fullUrl}`}
        >
          <Copy
            size={14}
            className={cn(
              'text-muted-foreground/50 transition-all duration-200',
              copied ? 'scale-0 opacity-0' : 'scale-100 opacity-100 group-hover/proxy:text-muted-foreground',
            )}
          />
          <Check
            size={16}
            className={cn(
              'absolute text-emerald-400 transition-all duration-200',
              copied ? 'scale-100 opacity-100' : 'scale-0 opacity-0',
            )}
          />
        </button>
      </div>
    </div>
  );
}
