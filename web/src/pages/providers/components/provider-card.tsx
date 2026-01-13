import {
  Wand2,
  Mail,
  Globe,
  Server,
  ChevronRight,
  Snowflake,
  X,
} from 'lucide-react'
import { ClientIcon } from '@/components/icons/client-icons'
import type { Provider } from '@/lib/transport'
import { ANTIGRAVITY_COLOR } from '../types'
import { useCooldowns } from '@/hooks/use-cooldowns'

interface ProviderCardProps {
  provider: Provider
  onClick: () => void
  streamingCount: number
}

export function AntigravityProviderCard({
  provider,
  onClick,
  streamingCount,
}: ProviderCardProps) {
  const email = provider.config?.antigravity?.email || 'Unknown'
  const {
    getCooldownForProvider,
    formatRemaining,
    clearCooldown,
    isClearingCooldown,
  } = useCooldowns()
  const cooldown = getCooldownForProvider(provider.id)

  const handleClearCooldown = (e: React.MouseEvent) => {
    e.stopPropagation() // Prevent triggering card onClick
    clearCooldown(provider.id)
  }

  return (
    <div
      onClick={onClick}
      className={`bg-surface-secondary border border-border rounded-xl p-4 hover:border-accent/30 hover:bg-surface-hover cursor-pointer transition-all relative group ${
        cooldown ? 'opacity-60' : ''
      }`}
    >
      {cooldown && (
        <div className="absolute top-3 right-3 flex items-center gap-1.5 px-2 py-1 rounded-md bg-cyan-500/20 border border-cyan-500/30">
          <Snowflake size={14} className="text-cyan-400 animate-pulse" />
          <span className="text-xs font-medium text-cyan-300">
            {formatRemaining(cooldown)}
          </span>
          <button
            onClick={handleClearCooldown}
            disabled={isClearingCooldown}
            className="ml-1 p-0.5 rounded hover:bg-cyan-500/30 transition-colors disabled:opacity-50"
            title="Clear cooldown"
          >
            <X size={12} className="text-cyan-300" />
          </button>
        </div>
      )}

      {!cooldown && streamingCount > 0 && (
        <div className="absolute top-3 right-3">
          <span
            className="px-1.5 py-0.5 rounded text-xs font-extrabold animate-pulse-soft shadow-md bg-gray-800 border"
            style={{
              borderColor: ANTIGRAVITY_COLOR,
              color: 'var(--color-primary-foreground)',
              boxShadow: `0 0 8px ${ANTIGRAVITY_COLOR}40`,
            }}
          >
            {streamingCount}
          </span>
        </div>
      )}

      <div className="flex items-start gap-3">
        <div
          className={`w-10 h-10 rounded-lg flex items-center justify-center shrink-0 ${
            cooldown ? 'bg-cyan-500/10' : ''
          }`}
          style={cooldown ? undefined : { backgroundColor: ANTIGRAVITY_COLOR }}
        >
          {cooldown ? (
            <Snowflake size={20} className="text-cyan-400" />
          ) : (
            <Wand2 size={20} style={{ color: ANTIGRAVITY_COLOR }} />
          )}
        </div>

        <div className="flex-1 min-w-0">
          <div className="flex items-center justify-between mb-1">
            <h4 className="text-sm font-medium text-text-primary truncate">
              {provider.name}
            </h4>
            <ChevronRight
              size={16}
              className="text-text-muted opacity-0 group-hover:opacity-100 transition-opacity"
            />
          </div>

          <div className="flex items-center gap-1.5 text-xs text-text-secondary mb-3">
            <Mail size={12} />
            <span className="truncate">{email}</span>
          </div>

          <div className="flex items-center gap-2">
            <span className="text-xs text-text-muted">Clients:</span>
            <div className="flex items-center gap-1">
              {provider.supportedClientTypes?.length > 0 ? (
                provider.supportedClientTypes.map(ct => (
                  <ClientIcon key={ct} type={ct} size={18} />
                ))
              ) : (
                <span className="text-xs text-text-muted">None</span>
              )}
            </div>
          </div>
        </div>
      </div>

      {!cooldown && streamingCount === 0 && (
        <div className="absolute top-3 right-3 w-2 h-2 rounded-full bg-emerald-400" />
      )}
    </div>
  )
}

export function CustomProviderCard({
  provider,
  onClick,
  streamingCount,
}: ProviderCardProps) {
  const {
    getCooldownForProvider,
    formatRemaining,
    clearCooldown,
    isClearingCooldown,
  } = useCooldowns()
  const cooldown = getCooldownForProvider(provider.id)

  const getDisplayUrl = () => {
    if (provider.config?.custom?.baseURL) return provider.config.custom.baseURL
    for (const ct of provider.supportedClientTypes || []) {
      const url = provider.config?.custom?.clientBaseURL?.[ct]
      if (url) return url
    }
    return 'Not configured'
  }

  const handleClearCooldown = (e: React.MouseEvent) => {
    e.stopPropagation() // Prevent triggering card onClick
    clearCooldown(provider.id)
  }

  return (
    <div
      onClick={onClick}
      className={`bg-surface-secondary border border-border rounded-xl p-4 hover:border-accent/30 hover:bg-surface-hover cursor-pointer transition-all relative group ${
        cooldown ? 'opacity-60' : ''
      }`}
    >
      {cooldown && (
        <div className="absolute top-3 right-3 flex items-center gap-1.5 px-2 py-1 rounded-md bg-cyan-500/20 border border-cyan-500/30">
          <Snowflake size={14} className="text-cyan-400 animate-pulse" />
          <span className="text-xs font-medium text-cyan-300">
            {formatRemaining(cooldown)}
          </span>
          <button
            onClick={handleClearCooldown}
            disabled={isClearingCooldown}
            className="ml-1 p-0.5 rounded hover:bg-cyan-500/30 transition-colors disabled:opacity-50"
            title="Clear cooldown"
          >
            <X size={12} className="text-cyan-300" />
          </button>
        </div>
      )}

      {!cooldown && streamingCount > 0 && (
        <div className="absolute top-3 right-3">
          <span
            className="px-1.5 py-0.5 rounded text-xs font-extrabold animate-pulse-soft shadow-md bg-gray-800 border"
            style={{
              borderColor: 'var(--color-accent)',
              color: '#FFF',
              boxShadow: '0 0 8px rgba(0, 120, 212, 0.4)',
            }}
          >
            {streamingCount}
          </span>
        </div>
      )}

      <div className="flex items-start gap-3">
        <div
          className={`w-10 h-10 rounded-lg flex items-center justify-center shrink-0 ${
            cooldown ? 'bg-cyan-500/10' : 'bg-surface-hover'
          }`}
        >
          {cooldown ? (
            <Snowflake size={20} className="text-cyan-400" />
          ) : (
            <Server size={20} className="text-text-secondary" />
          )}
        </div>

        <div className="flex-1 min-w-0">
          <div className="flex items-center justify-between mb-1">
            <h4 className="text-sm font-medium text-text-primary truncate">
              {provider.name}
            </h4>
            <ChevronRight
              size={16}
              className="text-text-muted opacity-0 group-hover:opacity-100 transition-opacity"
            />
          </div>

          <div className="flex items-center gap-1.5 text-xs text-text-secondary mb-3">
            <Globe size={12} />
            <span className="truncate">{getDisplayUrl()}</span>
          </div>

          <div className="flex items-center gap-2">
            <span className="text-xs text-text-muted">Clients:</span>
            <div className="flex items-center gap-1">
              {provider.supportedClientTypes?.length > 0 ? (
                provider.supportedClientTypes.map(ct => (
                  <ClientIcon key={ct} type={ct} size={18} />
                ))
              ) : (
                <span className="text-xs text-text-muted">None</span>
              )}
            </div>
          </div>
        </div>
      </div>

      {!cooldown && streamingCount === 0 && (
        <div className="absolute top-3 right-3 w-2 h-2 rounded-full bg-emerald-400" />
      )}
    </div>
  )
}
