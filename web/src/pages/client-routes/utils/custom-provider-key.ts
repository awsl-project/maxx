import type { Provider } from '@/lib/transport';

export function normalizeCustomProviderApiKeyInput(apiKey: string): string | null {
  const trimmed = apiKey.trim();
  return trimmed ? trimmed : null;
}

export function buildCustomProviderApiKeyUpdate(
  provider: Provider,
  apiKey: string,
): Pick<Provider, 'config'> {
  return {
    config: {
      ...(provider.config ?? {}),
      custom: {
        ...(provider.config?.custom ?? { baseURL: '', apiKey: '' }),
        apiKey,
      },
    },
  };
}

export function canQuickEditCustomProviderKey(provider: Provider): boolean {
  return provider.type === 'custom' && !!provider.config?.custom;
}
