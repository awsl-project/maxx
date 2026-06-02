import { Wand2, Mail, Globe, Server, ChevronRight, Snowflake, X } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { ClientIcon } from '@/components/icons/client-icons';
import { StreamingBadge } from '@/components/ui/streaming-badge';
import { CooldownTimer } from '@/components/cooldown-timer';
import type { Provider } from '@/lib/transport';
import { ANTIGRAVITY_COLOR, isOllamaCustomProvider } from '../types';
import { useCooldowns } from '@/hooks/use-cooldowns';

interface ProviderCardProps {
  provider: Provider;
  onClick: () => void;
  streamingCount: number;
}

export function AntigravityProviderCard({ provider, onClick, streamingCount }: ProviderCardProps) {
  const { t } = useTranslation();
  const email = provider.config?.antigravity?.email || t('provider.unknown');
  const { getCooldownsForProvider, getProviderHealthLevel, clearCooldown, isClearingCooldown } =
    useCooldowns();
  const providerCooldowns = getCooldownsForProvider(provider.id);
  const healthLevel = getProviderHealthLevel(provider.id);
  const modelCooldowns = providerCooldowns.filter((cd) => cd.model);
  const worstCooldown = providerCooldowns[0];
  const isFrozenOrLimited = healthLevel === 'frozen' || healthLevel === 'limited';

  const handleClearCooldown = (e: React.MouseEvent) => {
    e.stopPropagation(); // Prevent triggering card onClick
    clearCooldown(provider.id);
  };

  return (
    <div
      onClick={onClick}
      className={`bg-muted border border-border rounded-xl p-4 hover:border-accent/30 hover:bg-accent cursor-pointer transition-all relative group overflow-hidden ${
        healthLevel === 'frozen' ? 'opacity-60' : healthLevel === 'limited' ? 'opacity-80' : ''
      }`}
    >
      {healthLevel === 'degraded' && (
        <div className="absolute top-3 right-3 flex items-center gap-1.5 px-2 py-1 rounded-md bg-orange-500/20 border border-orange-500/30">
          <Snowflake size={14} className="text-orange-400" />
          <span className="text-xs font-medium text-orange-300">
            {modelCooldowns.length} model{modelCooldowns.length > 1 ? 's' : ''} frozen
          </span>
        </div>
      )}
      {healthLevel === 'limited' && worstCooldown && (
        <div className="absolute top-3 right-3 flex items-center gap-1.5 px-2 py-1 rounded-md bg-yellow-500/20 border border-yellow-500/30">
          <Snowflake size={14} className="text-yellow-400" />
          <CooldownTimer cooldown={worstCooldown} className="text-xs font-medium text-yellow-300" />
          <button
            onClick={handleClearCooldown}
            disabled={isClearingCooldown}
            className="ml-1 p-0.5 rounded hover:bg-yellow-500/30 transition-colors disabled:opacity-50"
            title={t('provider.clearCooldown')}
          >
            <X size={12} className="text-yellow-300" />
          </button>
        </div>
      )}
      {healthLevel === 'frozen' && worstCooldown && (
        <div className="absolute top-3 right-3 flex items-center gap-1.5 px-2 py-1 rounded-md bg-cyan-500/20 border border-cyan-500/30">
          <Snowflake size={14} className="text-cyan-400 animate-pulse" />
          <CooldownTimer cooldown={worstCooldown} className="text-xs font-medium text-cyan-300" />
          <button
            onClick={handleClearCooldown}
            disabled={isClearingCooldown}
            className="ml-1 p-0.5 rounded hover:bg-cyan-500/30 transition-colors disabled:opacity-50"
            title={t('provider.clearCooldown')}
          >
            <X size={12} className="text-cyan-300" />
          </button>
        </div>
      )}

      {(healthLevel === 'healthy' || healthLevel === 'degraded') && streamingCount > 0 && (
        <div className="absolute top-0 right-0 z-20">
          <StreamingBadge
            count={streamingCount}
            color={ANTIGRAVITY_COLOR}
            variant="corner"
            className="rounded-tr-xl rounded-bl-lg"
          />
        </div>
      )}

      <div className="flex items-start gap-3">
        <div
          className={`w-10 h-10 rounded-lg flex items-center justify-center shrink-0 ${
            isFrozenOrLimited ? 'bg-cyan-500/10' : 'bg-muted'
          }`}
        >
          {isFrozenOrLimited ? (
            <Snowflake size={20} className="text-cyan-400" />
          ) : (
            <Wand2 size={20} style={{ color: ANTIGRAVITY_COLOR }} />
          )}
        </div>

        <div className="flex-1 min-w-0">
          <div className="flex items-center justify-between mb-1">
            <h4 className="text-sm font-medium text-foreground truncate">{provider.name}</h4>
            <ChevronRight
              size={16}
              className="text-muted-foreground opacity-0 group-hover:opacity-100 transition-opacity"
            />
          </div>

          <div className="flex items-center gap-1.5 text-xs text-muted-foreground mb-3">
            <Mail size={12} />
            <span className="truncate">{email}</span>
          </div>

          <div className="flex items-center gap-2">
            <span className="text-xs text-text-muted">{t('provider.clients')}</span>
            <div className="flex items-center gap-1">
              {provider.supportedClientTypes?.length > 0 ? (
                provider.supportedClientTypes.map((ct) => (
                  <ClientIcon key={ct} type={ct} size={18} />
                ))
              ) : (
                <span className="text-xs text-text-muted">{t('provider.none')}</span>
              )}
            </div>
          </div>
        </div>
      </div>

      {healthLevel === 'healthy' && streamingCount === 0 && (
        <div className="absolute top-3 right-3 w-2 h-2 rounded-full bg-emerald-400" />
      )}
    </div>
  );
}

export function CustomProviderCard({ provider, onClick, streamingCount }: ProviderCardProps) {
  const { t } = useTranslation();
  const { getCooldownsForProvider, getProviderHealthLevel, clearCooldown, isClearingCooldown } =
    useCooldowns();
  const providerCooldowns = getCooldownsForProvider(provider.id);
  const healthLevel = getProviderHealthLevel(provider.id);
  const modelCooldowns = providerCooldowns.filter((cd) => cd.model);
  const worstCooldown = providerCooldowns[0];
  const isFrozenOrLimited = healthLevel === 'frozen' || healthLevel === 'limited';
  const isOllama = isOllamaCustomProvider(provider);

  const getDisplayUrl = () => {
    if (provider.config?.custom?.baseURL) return provider.config.custom.baseURL;
    for (const ct of provider.supportedClientTypes || []) {
      const url = provider.config?.custom?.clientBaseURL?.[ct];
      if (url) return url;
    }
    return t('provider.notConfigured');
  };

  const handleClearCooldown = (e: React.MouseEvent) => {
    e.stopPropagation(); // Prevent triggering card onClick
    clearCooldown(provider.id);
  };

  return (
    <div
      onClick={onClick}
      className={`bg-muted border border-border rounded-xl p-4 hover:border-accent/30 hover:bg-accent cursor-pointer transition-all relative group overflow-hidden ${
        healthLevel === 'frozen' ? 'opacity-60' : healthLevel === 'limited' ? 'opacity-80' : ''
      }`}
    >
      {healthLevel === 'degraded' && (
        <div className="absolute top-3 right-3 flex items-center gap-1.5 px-2 py-1 rounded-md bg-orange-500/20 border border-orange-500/30">
          <Snowflake size={14} className="text-orange-400" />
          <span className="text-xs font-medium text-orange-300">
            {modelCooldowns.length} model{modelCooldowns.length > 1 ? 's' : ''} frozen
          </span>
        </div>
      )}
      {healthLevel === 'limited' && worstCooldown && (
        <div className="absolute top-3 right-3 flex items-center gap-1.5 px-2 py-1 rounded-md bg-yellow-500/20 border border-yellow-500/30">
          <Snowflake size={14} className="text-yellow-400" />
          <CooldownTimer cooldown={worstCooldown} className="text-xs font-medium text-yellow-300" />
          <button
            onClick={handleClearCooldown}
            disabled={isClearingCooldown}
            className="ml-1 p-0.5 rounded hover:bg-yellow-500/30 transition-colors disabled:opacity-50"
            title={t('provider.clearCooldown')}
          >
            <X size={12} className="text-yellow-300" />
          </button>
        </div>
      )}
      {healthLevel === 'frozen' && worstCooldown && (
        <div className="absolute top-3 right-3 flex items-center gap-1.5 px-2 py-1 rounded-md bg-cyan-500/20 border border-cyan-500/30">
          <Snowflake size={14} className="text-cyan-400 animate-pulse" />
          <CooldownTimer cooldown={worstCooldown} className="text-xs font-medium text-cyan-300" />
          <button
            onClick={handleClearCooldown}
            disabled={isClearingCooldown}
            className="ml-1 p-0.5 rounded hover:bg-cyan-500/30 transition-colors disabled:opacity-50"
            title={t('provider.clearCooldown')}
          >
            <X size={12} className="text-cyan-300" />
          </button>
        </div>
      )}

      {(healthLevel === 'healthy' || healthLevel === 'degraded') && streamingCount > 0 && (
        <div className="absolute top-0 right-0 z-20">
          <StreamingBadge
            count={streamingCount}
            color="var(--color-accent)"
            variant="corner"
            className="rounded-tr-xl rounded-bl-lg"
          />
        </div>
      )}

      <div className="flex items-start gap-3">
        <div
          className={`w-10 h-10 rounded-lg flex items-center justify-center shrink-0 ${
            isFrozenOrLimited ? 'bg-cyan-500/10' : 'bg-muted'
          }`}
        >
          {isFrozenOrLimited ? (
            <Snowflake size={20} className="text-cyan-400" />
          ) : (
            <Server size={20} className="text-muted-foreground" />
          )}
        </div>

        <div className="flex-1 min-w-0">
          <div className="flex items-center justify-between mb-1">
            <div className="flex items-center gap-2 min-w-0">
              <h4 className="text-sm font-medium text-foreground truncate">{provider.name}</h4>
              {isOllama && (
                <span className="shrink-0 px-1.5 py-0.5 rounded-full text-[10px] font-bold bg-primary/10 text-primary border border-primary/20">
                  Ollama
                </span>
              )}
            </div>
            <ChevronRight
              size={16}
              className="text-muted-foreground opacity-0 group-hover:opacity-100 transition-opacity"
            />
          </div>

          <div className="flex items-center gap-1.5 text-xs text-muted-foreground mb-3">
            <Globe size={12} />
            <span className="truncate">{getDisplayUrl()}</span>
          </div>

          <div className="flex items-center gap-2">
            <span className="text-xs text-text-muted">{t('provider.clients')}</span>
            <div className="flex items-center gap-1">
              {provider.supportedClientTypes?.length > 0 ? (
                provider.supportedClientTypes.map((ct) => (
                  <ClientIcon key={ct} type={ct} size={18} />
                ))
              ) : (
                <span className="text-xs text-text-muted">{t('provider.none')}</span>
              )}
            </div>
          </div>
        </div>
      </div>

      {healthLevel === 'healthy' && streamingCount === 0 && (
        <div className="absolute top-3 right-3 w-2 h-2 rounded-full bg-emerald-400" />
      )}
    </div>
  );
}
