import type { AntigravityQuotaData, ProviderHealthLevel } from '@/lib/transport';

export type AntigravityAvailabilityStatus =
  | 'available'
  | 'low'
  | 'exhausted'
  | 'forbidden'
  | 'cooldown'
  | 'unknown';

export interface AntigravityAvailabilityInfo {
  status: AntigravityAvailabilityStatus;
  labelKey: string;
  descriptionKey: string;
  tone: 'success' | 'warning' | 'danger' | 'muted';
}

function findClaudePercentage(quota: AntigravityQuotaData | undefined): number | null {
  if (!quota?.models) return null;
  const claudeModel = quota.models.find((model) => model.name.toLowerCase().includes('claude'));
  return claudeModel?.percentage ?? null;
}

export function getAntigravityAvailabilityInfo(
  quota: AntigravityQuotaData | undefined,
  healthLevel?: ProviderHealthLevel,
): AntigravityAvailabilityInfo {
  if (healthLevel && healthLevel !== 'healthy') {
    return {
      status: 'cooldown',
      labelKey: 'providers.availability.cooldown',
      descriptionKey: 'providers.availability.cooldownDesc',
      tone: 'warning',
    };
  }

  if (!quota) {
    return {
      status: 'unknown',
      labelKey: 'providers.availability.unknown',
      descriptionKey: 'providers.availability.unknownDesc',
      tone: 'muted',
    };
  }

  if (quota.isForbidden) {
    return {
      status: 'forbidden',
      labelKey: 'providers.availability.forbidden',
      descriptionKey: 'providers.availability.forbiddenDesc',
      tone: 'danger',
    };
  }

  const claudePercentage = findClaudePercentage(quota);
  if (claudePercentage === null) {
    return {
      status: 'unknown',
      labelKey: 'providers.availability.unknown',
      descriptionKey: 'providers.availability.unknownDesc',
      tone: 'muted',
    };
  }

  if (claudePercentage <= 0) {
    return {
      status: 'exhausted',
      labelKey: 'providers.availability.exhausted',
      descriptionKey: 'providers.availability.exhaustedDesc',
      tone: 'danger',
    };
  }

  if (claudePercentage <= 20) {
    return {
      status: 'low',
      labelKey: 'providers.availability.low',
      descriptionKey: 'providers.availability.lowDesc',
      tone: 'warning',
    };
  }

  return {
    status: 'available',
    labelKey: 'providers.availability.available',
    descriptionKey: 'providers.availability.availableDesc',
    tone: 'success',
  };
}

export function getAntigravityAvailabilityBadgeClass(tone: AntigravityAvailabilityInfo['tone']) {
  switch (tone) {
    case 'success':
      return 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border-emerald-500/20';
    case 'warning':
      return 'bg-amber-500/10 text-amber-600 dark:text-amber-400 border-amber-500/20';
    case 'danger':
      return 'bg-destructive/10 text-destructive border-destructive/20';
    case 'muted':
    default:
      return 'bg-muted text-muted-foreground border-border/60';
  }
}
