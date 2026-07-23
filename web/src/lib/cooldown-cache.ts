import type { Cooldown } from './transport/types';

export type CooldownClearOptions = {
  clientType?: string;
  model?: string;
};

function normalizeScopeValue(value: string | undefined): string {
  return value ?? '';
}

export function removeClearedCooldowns(
  cooldowns: Cooldown[],
  providerId: number,
  options?: CooldownClearOptions,
): Cooldown[] {
  const clearAllForProvider = !options?.clientType && !options?.model;

  return cooldowns.filter((cooldown) => {
    if (cooldown.providerID !== providerId) return true;
    if (clearAllForProvider) return false;

    return !(
      normalizeScopeValue(cooldown.clientType) === normalizeScopeValue(options?.clientType) &&
      normalizeScopeValue(cooldown.model) === normalizeScopeValue(options?.model)
    );
  });
}
