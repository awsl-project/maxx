import { describe, expect, it } from 'vitest';

import type { ClaudeProviderBatchProviderResult } from '@/lib/transport';
import {
  collectSuccessfulRemovedExistingIDs,
  filterRemovedExistingResults,
  getClaudeBatchCandidateResultKey,
  getClaudeBatchExistingResultKey,
  getClaudeBatchOccurrenceMatchKeys,
  getClaudeBatchResultKey,
  getFailedExistingResultSignature,
  isClaudeBatchTestExcludedProvider,
  isClaudeBatchTestSelectableProvider,
  settleProviderRemovalsSequentially,
  summarizeClaudeBatchDisplayResults,
} from './claude-provider-batch-test';

function result(
  overrides: Partial<ClaudeProviderBatchProviderResult>,
): ClaudeProviderBatchProviderResult {
  return {
    index: 0,
    source: 'existing',
    existingID: 1,
    name: 'Provider A',
    type: 'custom',
    baseURL: 'https://example.com',
    requestedModel: 'claude-sonnet-4',
    mappedModel: 'claude-sonnet-4',
    action: 'test',
    status: 'usable',
    ok: true,
    persisted: false,
    routeCreated: false,
    routeUpdated: false,
    routeEnabled: false,
    durationMs: 1,
    ...overrides,
  };
}

describe('Claude provider batch test helpers', () => {
  it('excludes export-hidden providers from the Claude batch-test selectable list', () => {
    expect(
      isClaudeBatchTestSelectableProvider({
        type: 'custom',
        supportedClientTypes: ['claude'],
        excludeFromExport: false,
      }),
    ).toBe(true);
    expect(
      isClaudeBatchTestSelectableProvider({
        type: 'custom',
        supportedClientTypes: [],
        excludeFromExport: false,
      }),
    ).toBe(true);
    const exportHiddenClaudeProvider = {
      type: 'custom' as const,
      supportedClientTypes: ['claude'],
      excludeFromExport: true,
    };
    expect(isClaudeBatchTestSelectableProvider(exportHiddenClaudeProvider)).toBe(false);
    expect(isClaudeBatchTestExcludedProvider(exportHiddenClaudeProvider)).toBe(true);
    const openAIOnlyProvider = {
      type: 'custom' as const,
      supportedClientTypes: ['openai'],
      excludeFromExport: false,
    };
    expect(isClaudeBatchTestSelectableProvider(openAIOnlyProvider)).toBe(false);
    expect(isClaudeBatchTestExcludedProvider(openAIOnlyProvider)).toBe(false);
  });

  it('keys existing results by provider id and candidates by normalized name/base URL', () => {
    expect(getClaudeBatchExistingResultKey(42)).toBe('existing-42');
    expect(getClaudeBatchCandidateResultKey('  Provider X ', 'https://example.com///')).toBe(
      'candidate-provider x-https://example.com',
    );
    expect(getClaudeBatchResultKey(result({ source: 'existing', existingID: 7 }))).toBe(
      'existing-7',
    );
    expect(
      getClaudeBatchResultKey(
        result({
          source: 'candidate',
          existingID: undefined,
          name: 'New One',
          baseURL: 'https://api.test/',
        }),
      ),
    ).toBe('candidate-new one-https://api.test');
  });

  it('removes deleted existing providers from display results and recomputes summaries', () => {
    const results = [
      result({ existingID: 1, ok: false, persisted: false, routeCreated: false }),
      result({ existingID: 2, ok: true, persisted: true, routeCreated: true }),
      result({
        source: 'candidate',
        existingID: undefined,
        name: 'Candidate',
        ok: true,
        persisted: true,
        routeCreated: false,
      }),
    ];

    const displayResults = filterRemovedExistingResults(results, new Set([1]));

    expect(displayResults.map((item) => item.existingID ?? item.name)).toEqual([2, 'Candidate']);
    expect(summarizeClaudeBatchDisplayResults(displayResults)).toEqual({
      usableCount: 2,
      persistedCount: 2,
      routesCreated: 1,
    });
  });

  it('keeps failed removals selected by only collecting fulfilled delete targets', () => {
    const targets = [{ existingID: 1 }, { existingID: 2 }, { existingID: 3 }];
    const settled: PromiseSettledResult<unknown>[] = [
      { status: 'fulfilled', value: undefined },
      { status: 'rejected', reason: new Error('delete failed') },
      { status: 'fulfilled', value: undefined },
    ];

    expect(collectSuccessfulRemovedExistingIDs(targets, settled)).toEqual([1, 3]);
  });

  it('removes selected failed providers sequentially instead of firing a concurrent delete burst', async () => {
    const calls: number[] = [];
    let inFlight = 0;
    let maxInFlight = 0;

    const settled = await settleProviderRemovalsSequentially(
      [{ existingID: 10 }, { existingID: 20 }, { existingID: 30 }],
      async (providerID) => {
        inFlight += 1;
        maxInFlight = Math.max(maxInFlight, inFlight);
        calls.push(providerID);
        await Promise.resolve();
        inFlight -= 1;
      },
    );

    expect(calls).toEqual([10, 20, 30]);
    expect(maxInFlight).toBe(1);
    expect(settled.map((item) => item.status)).toEqual(['fulfilled', 'fulfilled', 'fulfilled']);
  });

  it('keeps successful sequential removals ordered when one provider delete fails', async () => {
    const targets = [{ existingID: 1 }, { existingID: 2 }, { existingID: 3 }];
    const settled = await settleProviderRemovalsSequentially(targets, async (providerID) => {
      if (providerID === 2) throw new Error('delete failed');
    });

    expect(settled.map((item) => item.status)).toEqual(['fulfilled', 'rejected', 'fulfilled']);
    expect(collectSuccessfulRemovedExistingIDs(targets, settled)).toEqual([1, 3]);
  });

  it('adds occurrence suffixes so duplicate candidate keys do not overwrite each other', () => {
    const candidateKey = getClaudeBatchCandidateResultKey('Same Provider', 'https://api.test/');

    expect(getClaudeBatchOccurrenceMatchKeys([candidateKey, candidateKey, 'existing-7'])).toEqual([
      `${candidateKey}#0`,
      `${candidateKey}#1`,
      'existing-7#0',
    ]);
  });

  it('builds a failed existing signature from the result identity and failure details', () => {
    expect(
      getFailedExistingResultSignature([
        result({ existingID: 1, status: 'request_failed', error: 'timeout', message: 'slow' }),
        result({ existingID: 2, status: 'upstream_5xx', error: '500', message: 'bad gateway' }),
      ]),
    ).toBe('1:request_failed:timeout:slow|2:upstream_5xx:500:bad gateway');
  });
});
