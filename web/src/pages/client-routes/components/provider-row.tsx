import { GripVertical, Zap, RefreshCw, Activity, Snowflake, Info, KeyRound } from 'lucide-react';
import {
  Button,
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  Input,
  Label,
  Switch,
} from '@/components/ui';
import { StreamingBadge } from '@/components/ui/streaming-badge';
import { MarqueeBackground } from '@/components/ui/marquee-background';
import { useSortable } from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { getProviderColorVar, type ProviderType } from '@/lib/theme';
import { cn } from '@/lib/utils';
import type {
  ClientType,
  ProviderStats,
  AntigravityQuotaData,
  ProviderHealthLevel,
  Route,
} from '@/lib/transport';
import { CooldownTimer } from '@/components/cooldown-timer';
import type { ProviderConfigItem } from '../types';
import { useAntigravityQuotaFromContext } from '@/contexts/antigravity-quotas-context';
import { useCooldownsContext } from '@/contexts/cooldowns-context';
import { useUpdateProvider, useUpdateRoute } from '@/hooks/queries';
import { ProviderDetailsDialog } from '@/components/provider-details-dialog';
import { useEffect, useRef, useState, memo } from 'react';
import { useTranslation } from 'react-i18next';
import {
  buildCustomProviderApiKeyUpdate,
  canQuickEditCustomProviderKey,
  normalizeCustomProviderApiKeyInput,
} from '../utils/custom-provider-key';

// Inline weight editor, shown on each route row only when the effective routing
// strategy is weighted_random. Commits on blur / Enter and is debounced by the
// fact that we only fire updateRoute when the value actually changes. Pointer
// events are stopped so editing never triggers the row's drag or details click.
function RouteWeightControlBase({ route }: { route: Route }) {
  const { t } = useTranslation();
  const updateRoute = useUpdateRoute();
  const [editing, setEditing] = useState(false);
  const [value, setValue] = useState(String(route.weight ?? 1));
  // Set by Escape so the blur it triggers cancels instead of committing. A ref
  // (not state) because blur fires synchronously within the keydown handler,
  // before any state update from this render would be visible to commit().
  const cancelRef = useRef(false);

  // Keep the input in sync with server state, but never clobber what the user
  // is mid-edit typing.
  useEffect(() => {
    if (!editing) setValue(String(route.weight ?? 1));
  }, [route.weight, editing]);

  const commit = () => {
    setEditing(false);
    if (cancelRef.current) {
      cancelRef.current = false;
      setValue(String(route.weight ?? 1));
      return;
    }
    let n = parseInt(value, 10);
    if (!Number.isFinite(n) || n < 1) n = 1;
    setValue(String(n));
    if (n !== (route.weight ?? 1)) {
      updateRoute.mutate({ id: route.id, data: { weight: n } });
    }
  };

  return (
    <div
      className="relative z-10 flex items-center gap-1.5 shrink-0 pl-1"
      onClick={(e) => e.stopPropagation()}
      onPointerDown={(e) => e.stopPropagation()}
      title={t('routes.weightTooltip')}
    >
      <span className="text-[9px] font-black text-muted-foreground/60 uppercase tracking-tight">
        {t('routes.weight')}
      </span>
      <input
        type="number"
        min={1}
        value={value}
        onFocus={() => setEditing(true)}
        onChange={(e) => setValue(e.target.value)}
        onBlur={commit}
        onKeyDown={(e) => {
          if (e.key === 'Enter') {
            (e.target as HTMLInputElement).blur();
          } else if (e.key === 'Escape') {
            cancelRef.current = true;
            (e.target as HTMLInputElement).blur();
          }
        }}
        disabled={updateRoute.isPending}
        className="w-12 h-7 rounded-md border border-border bg-background px-1.5 text-center text-[12px] font-mono font-bold tabular-nums focus:border-ring focus:ring-1 focus:ring-ring/50 outline-none disabled:opacity-50"
      />
    </div>
  );
}
const RouteWeightControl = memo(RouteWeightControlBase);

function CustomProviderKeyQuickEdit({ provider }: { provider: ProviderConfigItem['provider'] }) {
  const { t } = useTranslation();
  const updateProvider = useUpdateProvider();
  const [open, setOpen] = useState(false);
  const [apiKey, setApiKey] = useState('');
  const [error, setError] = useState<string | null>(null);

  const close = (nextOpen: boolean) => {
    setOpen(nextOpen);
    if (!nextOpen) {
      setApiKey('');
      setError(null);
    }
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    const nextKey = normalizeCustomProviderApiKeyInput(apiKey);
    if (!nextKey) {
      close(false);
      return;
    }

    setError(null);
    try {
      await updateProvider.mutateAsync({
        id: provider.id,
        data: buildCustomProviderApiKeyUpdate(provider, nextKey),
      });
      close(false);
    } catch (err) {
      console.error('Failed to update custom provider key:', err);
      setError(t('routes.customProviderKey.updateFailed'));
    }
  };

  return (
    <div
      className="relative z-10 flex items-center shrink-0"
      onClick={(e) => e.stopPropagation()}
      onPointerDown={(e) => e.stopPropagation()}
    >
      <div
        role="button"
        tabIndex={0}
        aria-disabled={updateProvider.isPending}
        data-testid={`custom-provider-key-edit-${provider.id}`}
        className={cn(
          'inline-flex h-6 items-center justify-center gap-1 rounded-full border border-transparent bg-muted/45 px-2 text-[10px] font-bold text-muted-foreground transition-colors hover:border-primary/20 hover:bg-primary/10 hover:text-foreground focus-visible:border-ring focus-visible:ring-ring/40 focus-visible:ring-[2px] outline-none',
          updateProvider.isPending && 'pointer-events-none opacity-50',
        )}
        onClick={() => close(true)}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault();
            close(true);
          }
        }}
        title={t('routes.customProviderKey.tooltip')}
      >
        <KeyRound size={11} />
        {t('routes.customProviderKey.button')}
      </div>
      <Dialog open={open} onOpenChange={close}>
        <DialogContent className="sm:max-w-md" onClick={(e) => e.stopPropagation()}>
          <form onSubmit={handleSubmit} className="space-y-4">
            <DialogHeader>
              <DialogTitle>{t('routes.customProviderKey.title')}</DialogTitle>
              <DialogDescription>
                {t('routes.customProviderKey.description', { name: provider.name })}
              </DialogDescription>
            </DialogHeader>
            <div className="space-y-2">
              <Label htmlFor={`custom-provider-key-${provider.id}`}>{t('provider.apiKey')}</Label>
              <Input
                id={`custom-provider-key-${provider.id}`}
                type="password"
                autoComplete="off"
                value={apiKey}
                placeholder={t('routes.customProviderKey.placeholder')}
                onChange={(e) => setApiKey(e.target.value)}
                autoFocus
              />
              <p className="text-xs text-muted-foreground">
                {t('routes.customProviderKey.scopeHint')}
              </p>
              {error && <p className="text-xs text-destructive">{error}</p>}
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => close(false)}>
                {t('provider.cancel')}
              </Button>
              <Button type="submit" disabled={updateProvider.isPending}>
                {updateProvider.isPending ? t('common.saving') : t('routes.customProviderKey.save')}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  );
}

// 格式化 Token 数量
function formatTokens(count: number): string {
  if (count >= 1_000_000) {
    return `${(count / 1_000_000).toFixed(1)}M`;
  }
  if (count >= 1_000) {
    return `${(count / 1_000).toFixed(1)}K`;
  }
  return count.toString();
}

// 格式化成本 (纳美元 → 美元，向下取整到 6 位)
function formatCost(nanoUsd: number): string {
  // 向下取整到 6 位小数 (microUSD 精度)
  const usd = Math.floor(nanoUsd / 1000) / 1_000_000;
  if (usd >= 1) {
    return `$${usd.toFixed(2)}`;
  }
  if (usd >= 0.01) {
    return `$${usd.toFixed(3)}`;
  }
  return `$${usd.toFixed(6).replace(/\.?0+$/, '')}`;
}

// Sortable Provider Row
type SortableProviderRowProps = {
  item: ProviderConfigItem;
  index: number;
  clientType: ClientType;
  streamingCount: number;
  stats?: ProviderStats;
  isToggling: boolean;
  showWeight?: boolean;
  onToggle: () => void;
  onDelete?: () => void;
};

function areSortableProviderRowEqual(
  prev: SortableProviderRowProps,
  next: SortableProviderRowProps,
) {
  return (
    prev.item === next.item &&
    prev.index === next.index &&
    prev.clientType === next.clientType &&
    prev.streamingCount === next.streamingCount &&
    prev.stats === next.stats &&
    prev.isToggling === next.isToggling &&
    prev.showWeight === next.showWeight
  );
}

function SortableProviderRowBase({
  item,
  index,
  clientType,
  streamingCount,
  stats,
  isToggling,
  showWeight,
  onToggle,
  onDelete,
}: SortableProviderRowProps) {
  const [showDetailsDialog, setShowDetailsDialog] = useState(false);
  const { getCooldownsForProvider, clearCooldown, isClearingCooldown } = useCooldownsContext();
  const activeCooldowns = getCooldownsForProvider(item.provider.id, clientType);

  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id: item.id,
    transition: {
      duration: 200,
      easing: 'ease',
    },
  });

  const style: React.CSSProperties = {
    transform: transform ? CSS.Translate.toString(transform) : undefined,
    transition,
    opacity: isDragging ? 0 : 1,
    pointerEvents: isDragging ? 'none' : undefined,
  };

  const handleRowClick = (e: React.MouseEvent) => {
    // 所有状态都打开详情弹窗
    e.stopPropagation();
    setShowDetailsDialog(true);
  };

  const handleClearCooldown = (options?: { clientType?: string; model?: string }) => {
    clearCooldown(item.provider.id, options);
  };

  return (
    <>
      <div ref={setNodeRef} style={style} {...attributes}>
        <ProviderRowContent
          item={item}
          index={index}
          clientType={clientType}
          streamingCount={streamingCount}
          stats={stats}
          isToggling={isToggling}
          showWeight={showWeight}
          onToggle={onToggle}
          onRowClick={handleRowClick}
          isInCooldown={activeCooldowns.length > 0}
          dragHandleListeners={listeners}
          onClearCooldown={handleClearCooldown}
          isClearingCooldown={isClearingCooldown}
        />
      </div>

      {/* Provider Details Dialog */}
      {showDetailsDialog && (
        <ProviderDetailsDialog
          item={item}
          clientType={clientType}
          open={showDetailsDialog}
          onOpenChange={setShowDetailsDialog}
          stats={stats}
          cooldowns={activeCooldowns}
          streamingCount={streamingCount}
          onToggle={onToggle}
          isToggling={isToggling}
          onDelete={onDelete}
          onClearCooldown={handleClearCooldown}
          isClearingCooldown={isClearingCooldown}
        />
      )}
    </>
  );
}

export const SortableProviderRow = memo(SortableProviderRowBase, areSortableProviderRowEqual);
SortableProviderRow.displayName = 'SortableProviderRow';

// Provider Row Content (used both in sortable and overlay)
type ProviderRowContentProps = {
  item: ProviderConfigItem;
  index: number;
  clientType: ClientType;
  streamingCount: number;
  stats?: ProviderStats;
  isToggling: boolean;
  isOverlay?: boolean;
  showWeight?: boolean;
  onToggle: () => void;
  onRowClick?: (e: React.MouseEvent) => void;
  isInCooldown?: boolean;
  dragHandleListeners?: any;
  onClearCooldown?: (options?: { clientType?: string; model?: string }) => void;
  isClearingCooldown?: boolean;
};

// 获取 Claude 模型额度百分比和重置时间
function getClaudeQuotaInfo(
  quota: AntigravityQuotaData | undefined,
): { percentage: number; resetTime: string; lastUpdated: number } | null {
  if (!quota || quota.isForbidden || !quota.models) return null;
  const claudeModel = quota.models.find((m) => m.name.toLowerCase().includes('claude'));
  if (!claudeModel) return null;
  return {
    percentage: claudeModel.percentage,
    resetTime: claudeModel.resetTime,
    lastUpdated: quota.lastUpdated,
  };
}

// 获取 Image 模型额度百分比
function getImageQuotaInfo(
  quota: AntigravityQuotaData | undefined,
): { percentage: number; resetTime: string } | null {
  if (!quota || quota.isForbidden || !quota.models) return null;
  const imageModel = quota.models.find((m) => m.name.toLowerCase().includes('image'));
  if (!imageModel) return null;
  return {
    percentage: imageModel.percentage,
    resetTime: imageModel.resetTime,
  };
}

// 格式化重置时间
function formatResetTime(resetTime: string, t: (key: string) => string): string {
  try {
    const reset = new Date(resetTime);
    const now = new Date();
    const diff = reset.getTime() - now.getTime();

    if (diff <= 0) return t('proxy.comingSoon');

    const hours = Math.floor(diff / (1000 * 60 * 60));
    const minutes = Math.floor((diff % (1000 * 60 * 60)) / (1000 * 60));

    if (hours > 24) {
      const days = Math.floor(hours / 24);
      return `${days}d`;
    }
    if (hours > 0) {
      return `${hours}h`;
    }
    return `${minutes}m`;
  } catch {
    return '-';
  }
}

// 格式化 lastUpdated 为相对时间
function formatLastUpdated(timestamp: number, t: (key: string) => string): string {
  if (!timestamp) return '';
  const now = Date.now();
  const diff = now - timestamp * 1000;
  const minutes = Math.floor(diff / (1000 * 60));

  if (minutes < 1) return t('common.now');
  if (minutes < 60) return `${minutes}m`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h`;
  const days = Math.floor(hours / 24);
  return `${days}d`;
}

function areProviderRowContentEqual(prev: ProviderRowContentProps, next: ProviderRowContentProps) {
  return (
    prev.item === next.item &&
    prev.index === next.index &&
    prev.clientType === next.clientType &&
    prev.streamingCount === next.streamingCount &&
    prev.stats === next.stats &&
    prev.isToggling === next.isToggling &&
    prev.isOverlay === next.isOverlay &&
    prev.showWeight === next.showWeight &&
    prev.isInCooldown === next.isInCooldown &&
    prev.isClearingCooldown === next.isClearingCooldown
  );
}

function ProviderRowContentBase({
  item,
  index,
  clientType,
  streamingCount,
  stats,
  isToggling,
  isOverlay: _isOverlay, // eslint-disable-line @typescript-eslint/no-unused-vars
  showWeight,
  onToggle,
  onRowClick,
  isInCooldown: isInCooldownProp,
  dragHandleListeners,
  onClearCooldown,
  isClearingCooldown,
}: ProviderRowContentProps) {
  const { t } = useTranslation();
  const { provider, enabled, isNative } = item;
  const color = getProviderColorVar(provider.type as ProviderType);
  const isAntigravity = provider.type === 'antigravity';

  // 从批量查询上下文获取 Antigravity 额度
  const quota = useAntigravityQuotaFromContext(provider.id);
  const claudeInfo = isAntigravity ? getClaudeQuotaInfo(quota) : null;
  const imageInfo = isAntigravity ? getImageQuotaInfo(quota) : null;

  // 获取 cooldown 状态
  const { getCooldownsForProvider } = useCooldownsContext();
  const providerCooldowns = getCooldownsForProvider(provider.id, clientType);
  const effectiveIsInCooldown = isInCooldownProp ?? providerCooldowns.length > 0;

  const healthLevel: ProviderHealthLevel =
    providerCooldowns.length === 0
      ? 'healthy'
      : providerCooldowns.some((cd) => !cd.clientType && !cd.model)
        ? 'frozen'
        : providerCooldowns.some((cd) => cd.clientType && !cd.model)
          ? 'limited'
          : 'degraded';

  const handleContentClick = (e: React.MouseEvent) => {
    // 所有状态都打开详情弹窗
    onRowClick?.(e);
  };

  return (
    <Button
      variant={null}
      onClick={handleContentClick}
      className={cn(
        'group relative flex flex-wrap items-center gap-x-4 gap-y-0 p-3 rounded-xl border transition-all duration-300 overflow-hidden w-full h-auto cursor-pointer active:cursor-grab',
        healthLevel === 'frozen' || healthLevel === 'limited'
          ? 'bg-transparent border-slate-400/50 dark:border-slate-500/40 hover:bg-slate-200/50 dark:hover:bg-slate-700/30 hover:border-slate-500 dark:hover:border-slate-400 hover:shadow-md'
          : enabled
            ? streamingCount > 0
              ? 'bg-accent/5 border-transparent ring-1 ring-black/5 dark:ring-white/10'
              : 'bg-card/60 border-border hover:border-emerald-500/30 hover:bg-card shadow-sm'
            : 'bg-muted/40 border-dashed border-border opacity-70 grayscale-[0.5] hover:opacity-100 hover:grayscale-0',
      )}
      style={{
        borderColor:
          healthLevel === 'frozen'
            ? 'rgb(6 182 212 / 0.3)'
            : healthLevel === 'limited'
              ? 'rgb(234 179 8 / 0.3)'
              : healthLevel === 'degraded'
                ? 'rgb(249 115 22 / 0.2)'
                : !effectiveIsInCooldown && enabled && streamingCount > 0
                  ? `${color}40`
                  : undefined,
        boxShadow:
          !effectiveIsInCooldown && enabled && streamingCount > 0
            ? `0 0 20px ${color}15`
            : undefined,
      }}
      {...dragHandleListeners}
    >
      <MarqueeBackground
        show={streamingCount > 0 && enabled && !effectiveIsInCooldown}
        color={`${color}15`}
        opacity={0.4}
      />

      {/* Cooldown 冰冻效果 - 落雪 (only for frozen/limited, NOT degraded) */}
      {(healthLevel === 'frozen' || healthLevel === 'limited') && (
        <>
          <div className="absolute inset-0 z-0 animate-snowing pointer-events-none opacity-80" />
          <div className="absolute inset-0 z-0 animate-snowing-secondary pointer-events-none opacity-80" />
        </>
      )}

      {/* Drag Handle & Index */}
      <div className="relative z-10 flex flex-col items-center gap-1.5 w-7 shrink-0">
        <div className="p-1 rounded-md hover:bg-accent transition-colors">
          <GripVertical
            size={14}
            className="text-muted-foreground group-hover:text-muted-foreground"
          />
        </div>
        <span
          className="text-[10px] font-mono font-bold w-5 h-5 flex items-center justify-center rounded-full border border-border bg-muted shadow-inner"
          style={{ color: enabled ? color : 'var(--color-text-muted)' }}
        >
          {index + 1}
        </span>
      </div>

      {/* Provider Main Info */}
      <div className="relative z-10 flex items-center gap-3 flex-1 min-w-0">
        {/* Icon */}
        <div
          className={cn(
            'relative w-11 h-11 rounded-xl flex items-center justify-center shrink-0 transition-all duration-500 overflow-hidden',
            healthLevel === 'frozen' || healthLevel === 'limited'
              ? 'bg-slate-200 dark:bg-slate-800 border border-slate-400/30 dark:border-slate-600/30'
              : 'bg-muted border border-border shadow-inner',
          )}
          style={
            (healthLevel === 'healthy' || healthLevel === 'degraded') && enabled ? { color } : {}
          }
        >
          <span
            className={cn(
              'text-xl font-black transition-all',
              healthLevel === 'frozen' || healthLevel === 'limited'
                ? 'opacity-0'
                : enabled
                  ? 'scale-100'
                  : 'opacity-30 grayscale',
            )}
          >
            {provider.name.charAt(0).toUpperCase()}
          </span>
          {(healthLevel === 'frozen' || healthLevel === 'limited') && (
            <Snowflake
              size={22}
              className="absolute text-slate-500/70 dark:text-white/70 animate-pulse drop-shadow-[0_0_8px_rgba(100,116,139,0.4)] dark:drop-shadow-[0_0_8px_rgba(255,255,255,0.4)]"
            />
          )}
          {enabled && streamingCount > 0 && healthLevel === 'healthy' && (
            <div className="absolute inset-0 bg-black/5 dark:bg-white/5 animate-pulse" />
          )}
        </div>

        {/* Text Info */}
        <div className="flex flex-col min-w-0">
          <div className="flex items-center gap-2">
            <span
              className={cn(
                'text-[14px] font-bold truncate transition-colors',
                effectiveIsInCooldown
                  ? 'text-foreground'
                  : enabled
                    ? 'text-foreground'
                    : 'text-muted-foreground',
              )}
            >
              {provider.name}
            </span>
            {/* Badges */}
            <div className="flex items-center gap-1.5 shrink-0">
              {isNative ? (
                <span className="flex items-center gap-1 px-1.5 py-0.5 rounded-full text-[10px] font-bold bg-emerald-500/10 text-emerald-500 border border-emerald-500/20">
                  <Zap size={10} className="fill-emerald-500/20" /> NATIVE
                </span>
              ) : (
                <span className="flex items-center gap-1 px-1.5 py-0.5 rounded-full text-[10px] font-bold bg-amber-500/10 text-amber-500 border border-amber-500/20">
                  <RefreshCw size={10} /> CONV
                </span>
              )}
            </div>
          </div>
          <div className="flex items-center gap-3">
            {/* 对于 Antigravity，显示 Claude 和 Imagen Quota；对于其他类型，显示 endpoint */}
            {isAntigravity && (claudeInfo || imageInfo) ? (
              <div className={cn('flex items-center gap-3 shrink-0', !enabled && 'opacity-40')}>
                {/* Claude Quota */}
                {claudeInfo && (
                  <div className="flex items-center gap-1.5">
                    <span className="text-[9px] font-black text-muted-foreground/60 uppercase">
                      Claude
                    </span>
                    <div className="w-14 h-1.5 bg-muted rounded-full overflow-hidden border border-border/50">
                      <div
                        className={cn(
                          'h-full rounded-full transition-all duration-1000',
                          claudeInfo.percentage >= 50
                            ? 'bg-emerald-500'
                            : claudeInfo.percentage >= 20
                              ? 'bg-amber-500'
                              : 'bg-red-500',
                        )}
                        style={{ width: `${claudeInfo.percentage}%` }}
                      />
                    </div>
                    <span className="text-[9px] font-mono text-muted-foreground/60">
                      {formatResetTime(claudeInfo.resetTime, t)}
                    </span>
                  </div>
                )}
                {/* Image Quota */}
                {imageInfo && (
                  <div className="flex items-center gap-1.5">
                    <span className="text-[9px] font-black text-muted-foreground/60 uppercase">
                      {t('common.image')}
                    </span>
                    <div className="w-14 h-1.5 bg-muted rounded-full overflow-hidden border border-border/50">
                      <div
                        className={cn(
                          'h-full rounded-full transition-all duration-1000',
                          imageInfo.percentage >= 50
                            ? 'bg-emerald-500'
                            : imageInfo.percentage >= 20
                              ? 'bg-amber-500'
                              : 'bg-red-500',
                        )}
                        style={{ width: `${imageInfo.percentage}%` }}
                      />
                    </div>
                    <span className="text-[9px] font-mono text-muted-foreground/60">
                      {formatResetTime(imageInfo.resetTime, t)}
                    </span>
                  </div>
                )}
                {/* Last Updated */}
                {claudeInfo && (
                  <span
                    className="text-[8px] font-mono text-muted-foreground/40"
                    title={t('common.lastUpdated')}
                  >
                    @{formatLastUpdated(claudeInfo.lastUpdated, t)}
                  </span>
                )}
              </div>
            ) : (
              <div
                className={cn(
                  'text-[11px] font-medium truncate flex items-center gap-1',
                  effectiveIsInCooldown
                    ? 'text-muted-foreground'
                    : enabled
                      ? 'text-muted-foreground'
                      : 'text-muted-foreground/50',
                )}
              >
                <Info size={10} className="shrink-0" />
                {provider.config?.custom?.clientBaseURL?.[clientType] ||
                  provider.config?.custom?.baseURL ||
                  provider.config?.antigravity?.endpoint ||
                  t('provider.defaultEndpoint')}
              </div>
            )}
          </div>
        </div>
      </div>

      {/* Stats Grid */}
      <div
        className={cn(
          'relative z-10 flex items-center gap-px bg-muted/50 rounded-xl border border-border/60 p-0.5 backdrop-blur-sm shrink-0',
          !enabled && 'opacity-40',
        )}
      >
        {stats && stats.totalRequests > 0 ? (
          <>
            <div className="flex flex-col items-center min-w-[50px] px-2 py-1">
              <span className="text-[8px] font-bold text-muted-foreground uppercase tracking-tight">
                SR
              </span>
              <span
                className={cn(
                  'font-mono font-black text-[12px]',
                  stats.successRate >= 95
                    ? 'text-emerald-500'
                    : stats.successRate >= 90
                      ? 'text-blue-400'
                      : 'text-amber-500',
                )}
              >
                {Math.round(stats.successRate)}%
              </span>
            </div>
            <div className="w-[1px] h-6 bg-border/40" />
            <div className="flex flex-col items-center min-w-[50px] px-2 py-1">
              <span className="text-[8px] font-bold text-muted-foreground uppercase tracking-tight">
                TKN
              </span>
              <span className="font-mono font-black text-[12px] text-blue-400">
                {formatTokens(stats.totalInputTokens + stats.totalOutputTokens)}
              </span>
            </div>
            <div className="w-[1px] h-6 bg-border/40" />
            <div className="flex flex-col items-center min-w-[55px] px-2 py-1">
              <span className="text-[8px] font-bold text-muted-foreground uppercase tracking-tight">
                {t('common.cost')}
              </span>
              <span className="font-mono font-black text-[12px] text-purple-400">
                {formatCost(stats.totalCost)}
              </span>
            </div>
          </>
        ) : (
          <div className="px-6 py-2 flex items-center gap-2 text-muted-foreground/30">
            <Activity size={12} />
            <span className="text-[10px] font-bold uppercase tracking-widest">
              {t('common.noData')}
            </span>
          </div>
        )}
      </div>
      {/* Streaming Indicator */}
      {enabled && streamingCount > 0 && !effectiveIsInCooldown && (
        <div className="relative z-10 flex items-center shrink-0">
          <StreamingBadge count={streamingCount} color={color} />
        </div>
      )}
      {/* Custom provider key quick edit: route UI edits provider-level key only. */}
      {item.route && canQuickEditCustomProviderKey(provider) && (
        <CustomProviderKeyQuickEdit provider={provider} />
      )}
      {/* Weight editor (weighted_random strategy only) */}
      {showWeight && item.route && <RouteWeightControl route={item.route} />}
      {/* Switch */}
      <div
        className="relative z-10 flex items-center shrink-0 pl-2"
        onClick={(e) => e.stopPropagation()}
        onPointerDown={(e) => e.stopPropagation()}
      >
        <Switch checked={enabled} onCheckedChange={onToggle} disabled={isToggling} />
      </div>

      {/* Cooldown Section — expands below main row when cooldowns exist */}
      {effectiveIsInCooldown && providerCooldowns.length > 0 && (
        <div className="relative z-10 w-full mt-1 pt-2 border-t border-dashed border-current/10">
          <div className="flex flex-wrap gap-2">
            {providerCooldowns.map((cd, i) => {
              const isFrozen = !cd.clientType && !cd.model;
              const isLimited = cd.clientType && !cd.model;
              return (
                <div
                  key={i}
                  className={cn(
                    'flex items-center gap-2 px-2.5 py-1 rounded-lg border text-xs',
                    isFrozen && 'bg-cyan-500/10 border-cyan-500/20',
                    isLimited && 'bg-yellow-500/10 border-yellow-500/20',
                    !isFrozen && !isLimited && 'bg-orange-500/10 border-orange-500/20',
                  )}
                >
                  <Snowflake
                    size={11}
                    className={cn(
                      isFrozen && 'text-cyan-500 animate-pulse',
                      isLimited && 'text-yellow-500',
                      !isFrozen && !isLimited && 'text-orange-500',
                    )}
                  />
                  <span
                    className={cn(
                      'font-medium text-[11px]',
                      isFrozen && 'text-cyan-500',
                      isLimited && 'text-yellow-500',
                      !isFrozen && !isLimited && 'text-orange-400',
                    )}
                  >
                    {cd.model || (isFrozen ? 'Provider' : cd.clientType || 'Key')}
                  </span>
                  <CooldownTimer
                    cooldown={cd}
                    className={cn(
                      'font-mono font-bold text-[11px] tabular-nums',
                      isFrozen && 'text-cyan-400',
                      isLimited && 'text-yellow-400',
                      !isFrozen && !isLimited && 'text-orange-500',
                    )}
                  />
                  <button
                    onClick={(e) => {
                      e.stopPropagation();
                      onClearCooldown?.({
                        clientType: cd.clientType || undefined,
                        model: cd.model || undefined,
                      });
                    }}
                    disabled={isClearingCooldown}
                    className="p-0.5 rounded hover:bg-accent/50 transition-colors disabled:opacity-50"
                    title={t('provider.clearCooldown')}
                  >
                    <Zap size={10} className="text-muted-foreground/50" />
                  </button>
                </div>
              );
            })}
          </div>
        </div>
      )}
    </Button>
  );
}

export const ProviderRowContent = memo(ProviderRowContentBase, areProviderRowContentEqual);
ProviderRowContent.displayName = 'ProviderRowContent';
