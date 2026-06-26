import type { ClaudeProviderBatchProviderResult } from '@/lib/transport';

export function getClaudeBatchExistingResultKey(providerID: number) {
  return `existing-${providerID}`;
}

export function getClaudeBatchCandidateResultKey(name: string, baseURL: string) {
  return `candidate-${name.trim().toLowerCase()}-${baseURL.replace(/\/+$/, '')}`;
}

export function getClaudeBatchResultKey(
  result: Pick<ClaudeProviderBatchProviderResult, 'source' | 'existingID' | 'name' | 'baseURL'>,
) {
  if (result.source === 'existing' && result.existingID) {
    return getClaudeBatchExistingResultKey(result.existingID);
  }
  return getClaudeBatchCandidateResultKey(result.name, result.baseURL ?? '');
}

export function filterRemovedExistingResults(
  results: ClaudeProviderBatchProviderResult[] | undefined,
  removedExistingIDs: Set<number>,
) {
  return (results ?? []).filter(
    (result) =>
      result.source !== 'existing' ||
      !result.existingID ||
      !removedExistingIDs.has(result.existingID),
  );
}

export function summarizeClaudeBatchDisplayResults(results: ClaudeProviderBatchProviderResult[]) {
  return {
    usableCount: results.filter((result) => result.ok).length,
    persistedCount: results.filter((result) => result.persisted).length,
    routesCreated: results.filter((result) => result.routeCreated).length,
  };
}

export function collectSuccessfulRemovedExistingIDs<T extends { existingID?: number }>(
  targets: T[],
  settled: PromiseSettledResult<unknown>[],
) {
  return targets
    .filter((target, index) => Boolean(target.existingID) && settled[index]?.status === 'fulfilled')
    .map((target) => target.existingID!);
}
