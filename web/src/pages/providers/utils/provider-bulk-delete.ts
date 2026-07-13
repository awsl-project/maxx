import type { Provider, ProviderBulkDeleteResult } from '@/lib/transport';
import { normalizeProviderArrayField } from './provider-normalize';

export type ProviderBulkDeleteStatus = {
  deleted: number;
  failed: Array<{ id: number; name: string; message: string }>;
};

export function buildProviderBulkDeleteStatus(
  selectedProviders: Provider[],
  result: ProviderBulkDeleteResult,
  notDeletedMessage: string,
): ProviderBulkDeleteStatus {
  const selectedProviderIDs = new Set(selectedProviders.map((provider) => provider.id));
  const normalizedDeletedIDs = normalizeProviderArrayField(result.deletedIDs);
  const inferredDeletedIDs =
    normalizedDeletedIDs.length === 0 && result.deletedCount >= selectedProviders.length
      ? selectedProviders.map((provider) => provider.id)
      : normalizedDeletedIDs;
  const deletedIDs = new Set(inferredDeletedIDs);
  const notFoundIDs = normalizeProviderArrayField(result.notFoundIDs);
  const resolvedMissingIDs = new Set(
    notFoundIDs.filter((id) => selectedProviderIDs.has(id) && !deletedIDs.has(id)),
  );
  const resolvedIDs = new Set([...deletedIDs, ...resolvedMissingIDs]);

  return {
    deleted: result.deletedCount + resolvedMissingIDs.size,
    failed: selectedProviders
      .filter((provider) => !resolvedIDs.has(provider.id))
      .map((provider) => ({
        id: provider.id,
        name: provider.name,
        message: notDeletedMessage,
      })),
  };
}
