import type {
  ClaudeProviderBatchProviderResult,
  Provider,
  ProviderBulkDeleteResult,
} from '@/lib/transport';

type ClaudeBatchProviderSelectionFields = Pick<
  Provider,
  'type' | 'supportedClientTypes' | 'excludeFromExport'
>;

function supportsClaudeBatchTest(provider: ClaudeBatchProviderSelectionFields) {
  return (
    provider.type === 'custom' &&
    (provider.supportedClientTypes?.length === 0 ||
      provider.supportedClientTypes?.includes('claude'))
  );
}

export function isClaudeBatchTestSelectableProvider(provider: ClaudeBatchProviderSelectionFields) {
  return supportsClaudeBatchTest(provider) && !provider.excludeFromExport;
}

export function isClaudeBatchTestExcludedProvider(provider: ClaudeBatchProviderSelectionFields) {
  return supportsClaudeBatchTest(provider) && !!provider.excludeFromExport;
}

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

export function getClaudeBatchResultMatchKey(baseKey: string, occurrence: number) {
  return `${baseKey}#${occurrence}`;
}

export function getClaudeBatchOccurrenceMatchKeys(baseKeys: string[]) {
  const occurrenceByBaseKey = new Map<string, number>();
  return baseKeys.map((baseKey) => {
    const occurrence = occurrenceByBaseKey.get(baseKey) ?? 0;
    occurrenceByBaseKey.set(baseKey, occurrence + 1);
    return getClaudeBatchResultMatchKey(baseKey, occurrence);
  });
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

export async function settleProviderRemovalsInBulk<T extends { existingID?: number }>(
  targets: T[],
  removeProviders: (providerIDs: number[]) => Promise<ProviderBulkDeleteResult>,
): Promise<PromiseSettledResult<unknown>[]> {
  const providerIDs = targets.map((target) => target.existingID).filter((id): id is number => !!id);
  if (providerIDs.length === 0) {
    return targets.map(() => ({
      status: 'rejected',
      reason: new Error('missing existing provider id'),
    }));
  }

  let result: ProviderBulkDeleteResult;
  try {
    result = await removeProviders(providerIDs);
  } catch (reason) {
    return targets.map((target) => ({
      status: 'rejected',
      reason: target.existingID ? reason : new Error('missing existing provider id'),
    }));
  }
  const deletedIDs = new Set(result.deletedIDs);
  const notFoundIDs = new Set(result.notFoundIDs);

  return targets.map((target) => {
    if (!target.existingID) {
      return { status: 'rejected', reason: new Error('missing existing provider id') };
    }
    if (deletedIDs.has(target.existingID)) {
      return { status: 'fulfilled', value: result };
    }
    const reason = notFoundIDs.has(target.existingID)
      ? new Error(`provider ${target.existingID} not found`)
      : new Error(`provider ${target.existingID} was not deleted`);
    return { status: 'rejected', reason };
  });
}

export function getFailedExistingResultSignature(
  results: Pick<ClaudeProviderBatchProviderResult, 'existingID' | 'status' | 'error' | 'message'>[],
) {
  return results
    .map(
      (result) =>
        `${result.existingID ?? 0}:${result.status ?? ''}:${result.error ?? ''}:${result.message ?? ''}`,
    )
    .join('|');
}
