import { describe, expect, it } from 'vitest';
import type { Provider } from '@/lib/transport';
import {
  buildCustomProviderApiKeyUpdate,
  canQuickEditCustomProviderKey,
  normalizeCustomProviderApiKeyInput,
} from './custom-provider-key';

const baseProvider: Provider = {
  id: 1,
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
  type: 'custom',
  name: 'Custom Relay',
  config: {
    disableErrorCooldown: true,
    custom: {
      baseURL: 'https://relay.example/v1',
      apiKey: 'old-key',
      backend: 'ollama',
      clientBaseURL: { claude: 'https://relay.example/claude' },
      clientMultiplier: { claude: 12000 },
      modelMapping: { '*': 'upstream-model' },
      responseModelMapping: { upstream: 'client' },
      responsesPassthrough: false,
    },
  },
  supportedClientTypes: ['claude'],
};

describe('custom provider key quick edit helpers', () => {
  it('updates only the custom provider api key while preserving existing config', () => {
    const update = buildCustomProviderApiKeyUpdate(baseProvider, 'new-key');

    expect(update.config?.disableErrorCooldown).toBe(true);
    expect(update.config?.custom).toEqual({
      ...baseProvider.config?.custom,
      apiKey: 'new-key',
    });
  });

  it('allows quick edit only for configured custom providers', () => {
    expect(canQuickEditCustomProviderKey(baseProvider)).toBe(true);
    expect(canQuickEditCustomProviderKey({ ...baseProvider, type: 'codex' })).toBe(false);
    expect(canQuickEditCustomProviderKey({ ...baseProvider, config: null })).toBe(false);
  });

  it('treats a blank quick-edit key as no change', () => {
    expect(normalizeCustomProviderApiKeyInput('')).toBeNull();
    expect(normalizeCustomProviderApiKeyInput('   ')).toBeNull();
    expect(normalizeCustomProviderApiKeyInput('\n\t')).toBeNull();
    expect(normalizeCustomProviderApiKeyInput('  new-key  ')).toBe('new-key');
  });
});
