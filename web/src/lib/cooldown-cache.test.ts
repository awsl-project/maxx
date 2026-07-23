import { describe, expect, it } from 'vitest';
import type { Cooldown } from './transport/types';
import { removeClearedCooldowns } from './cooldown-cache';

const baseCooldown = {
  until: '2099-01-01T00:00:00Z',
  reason: 'manual',
  providerName: 'provider',
} satisfies Omit<Cooldown, 'providerID'>;

function cooldown(providerID: number, clientType?: string, model?: string): Cooldown {
  return {
    ...baseCooldown,
    providerID,
    clientType,
    model,
  };
}

describe('removeClearedCooldowns', () => {
  it('removes all cooldowns for a provider when no scope is given', () => {
    const result = removeClearedCooldowns([
      cooldown(1),
      cooldown(1, 'openai'),
      cooldown(1, 'openai', 'gpt-5'),
      cooldown(2),
    ], 1);

    expect(result).toEqual([cooldown(2)]);
  });

  it('removes only the matching client cooldown', () => {
    const providerLevel = cooldown(1);
    const openAI = cooldown(1, 'openai');
    const claude = cooldown(1, 'claude');
    const modelLevel = cooldown(1, 'openai', 'gpt-5');

    const result = removeClearedCooldowns([providerLevel, openAI, claude, modelLevel], 1, {
      clientType: 'openai',
    });

    expect(result).toEqual([providerLevel, claude, modelLevel]);
  });

  it('removes only the matching client and model cooldown', () => {
    const providerLevel = cooldown(1);
    const keyLevel = cooldown(1, 'openai');
    const targetModel = cooldown(1, 'openai', 'gpt-5');
    const otherModel = cooldown(1, 'openai', 'gpt-4');

    const result = removeClearedCooldowns([providerLevel, keyLevel, targetModel, otherModel], 1, {
      clientType: 'openai',
      model: 'gpt-5',
    });

    expect(result).toEqual([providerLevel, keyLevel, otherModel]);
  });

  it('removes only the matching model-only cooldown', () => {
    const providerLevel = cooldown(1);
    const keyLevel = cooldown(1, 'openai');
    const targetModel = cooldown(1, undefined, 'gpt-5');
    const clientModel = cooldown(1, 'openai', 'gpt-5');

    const result = removeClearedCooldowns([providerLevel, keyLevel, targetModel, clientModel], 1, {
      model: 'gpt-5',
    });

    expect(result).toEqual([providerLevel, keyLevel, clientModel]);
  });
});
