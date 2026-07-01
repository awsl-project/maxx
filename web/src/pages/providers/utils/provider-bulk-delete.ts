import type { Provider, ProviderBulkDeleteResult } from '@/lib/transport';

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
  const deletedIDs = new Set(result.deletedIDs);
  const resolvedMissingIDs = new Set(
    result.notFoundIDs.filter((id) => selectedProviderIDs.has(id) && !deletedIDs.has(id)),
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
