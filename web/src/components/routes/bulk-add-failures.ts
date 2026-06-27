import type { Provider } from '@/lib/transport';

export interface BulkAddRouteFailure {
  providerID: number;
  providerName: string;
  providerType: string;
  message: string;
}

function readStringField(value: unknown, field: string): string | null {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return null;
  const record = value as Record<string, unknown>;
  const fieldValue = record[field];
  return typeof fieldValue === 'string' && fieldValue.trim() ? fieldValue.trim() : null;
}

export function getBulkAddRouteFailureMessage(error: unknown): string {
  if (!error) return 'Unknown error';

  const responseData = (error as { response?: { data?: unknown } }).response?.data;
  const responseMessage =
    readStringField(responseData, 'message') ??
    readStringField(responseData, 'error') ??
    readStringField(responseData, 'detail');
  if (responseMessage) return responseMessage;

  if (typeof responseData === 'string' && responseData.trim()) return responseData.trim();

  const directMessage = readStringField(error, 'message');
  if (directMessage) return directMessage;

  try {
    return JSON.stringify(error);
  } catch {
    return String(error);
  }
}

export function createBulkAddRouteFailure(provider: Provider, error: unknown): BulkAddRouteFailure {
  return {
    providerID: Number(provider.id),
    providerName: provider.name,
    providerType: provider.type,
    message: getBulkAddRouteFailureMessage(error),
  };
}
