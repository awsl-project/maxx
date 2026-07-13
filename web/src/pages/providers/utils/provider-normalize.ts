import type { Provider } from '@/lib/transport';

export function normalizeProviderList(providers: Provider[] | null | undefined): Provider[] {
  return Array.isArray(providers) ? providers : [];
}

export function normalizeProviderArrayField<T>(value: T[] | null | undefined): T[] {
  return Array.isArray(value) ? value : [];
}
