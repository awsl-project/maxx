import { describe, expect, it } from 'vitest';
import type { ProxyRequest } from '@/lib/transport';
import {
  isProxyRequestError,
  mergeProxyRequestAttemptUpdate,
  shouldRefetchCleanupFailedCount,
} from './use-requests';

const baseRequest = {
  id: 1,
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
  instanceID: 'instance-1',
  requestID: 'request-1',
  sessionID: 'session-1',
  clientType: 'openai',
  requestModel: 'model-a',
  mappedModel: 'model-a',
  responseModel: '',
  reasoningEffort: '',
  startTime: '2026-01-01T00:00:00Z',
  endTime: '2026-01-01T00:00:01Z',
  duration: 1,
  ttft: 0,
  isStream: false,
  statusCode: 200,
  requestInfo: null,
  responseInfo: null,
  error: '',
  proxyUpstreamAttemptCount: 0,
  finalProxyUpstreamAttemptID: 0,
  routeID: 0,
  providerID: 0,
  projectID: 0,
  inputTokenCount: 0,
  outputTokenCount: 0,
  cacheReadCount: 0,
  cacheWriteCount: 0,
  cache5mWriteCount: 0,
  cache1hWriteCount: 0,
  modelPriceId: 0,
  multiplier: 10000,
  cost: 0,
  apiTokenID: 0,
} satisfies Omit<ProxyRequest, 'status'>;

function request(status: ProxyRequest['status'], statusCode = 200): ProxyRequest {
  return { ...baseRequest, status, statusCode };
}

describe('isProxyRequestError', () => {
  it('treats cancelled requests as error records', () => {
    expect(isProxyRequestError(request('CANCELLED'))).toBe(true);
  });

  it('keeps completed 2xx requests out of error records', () => {
    expect(isProxyRequestError(request('COMPLETED'))).toBe(false);
  });

  it('treats rejected, failed, and HTTP error responses as error records', () => {
    expect(isProxyRequestError(request('FAILED'))).toBe(true);
    expect(isProxyRequestError(request('REJECTED'))).toBe(true);
    expect(isProxyRequestError(request('COMPLETED', 503))).toBe(true);
  });
});

describe('shouldRefetchCleanupFailedCount', () => {
  it('refetches when a new request is an error record', () => {
    expect(shouldRefetchCleanupFailedCount(undefined, request('FAILED'))).toBe(true);
  });

  it('does not refetch when a new request is not an error record', () => {
    expect(shouldRefetchCleanupFailedCount(undefined, request('COMPLETED'))).toBe(false);
  });

  it('refetches when an existing request leaves the cleanup-failed set', () => {
    expect(shouldRefetchCleanupFailedCount(request('FAILED'), request('COMPLETED'))).toBe(true);
  });

  it('refetches when an existing request enters the cleanup-failed set', () => {
    expect(shouldRefetchCleanupFailedCount(request('IN_PROGRESS'), request('COMPLETED', 500))).toBe(
      true,
    );
  });

  it('does not refetch when the cleanup-failed membership stays unchanged', () => {
    expect(shouldRefetchCleanupFailedCount(request('FAILED'), request('COMPLETED', 503))).toBe(
      false,
    );
    expect(shouldRefetchCleanupFailedCount(request('IN_PROGRESS'), request('COMPLETED'))).toBe(
      false,
    );
  });
});

describe('mergeProxyRequestAttemptUpdate', () => {
  const attempt = {
    id: 11,
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:02Z',
    startTime: '2026-01-01T00:00:00Z',
    endTime: '2026-01-01T00:00:02Z',
    duration: 2,
    ttft: 123,
    status: 'IN_PROGRESS',
    error: '',
    proxyRequestID: 1,
    isStream: true,
    requestModel: 'model-a',
    mappedModel: 'mapped-live',
    responseModel: 'response-live',
    requestInfo: null,
    responseInfo: null,
    routeID: 7,
    providerID: 9,
    inputTokenCount: 101,
    outputTokenCount: 202,
    cacheReadCount: 303,
    cacheWriteCount: 404,
    cache5mWriteCount: 50,
    cache1hWriteCount: 60,
    modelPriceId: 12,
    multiplier: 15000,
    cost: 999,
  } as const;

  it('mirrors live attempt provider and token fields onto the request summary', () => {
    const merged = mergeProxyRequestAttemptUpdate(request('IN_PROGRESS'), attempt);

    expect(merged.providerID).toBe(9);
    expect(merged.routeID).toBe(7);
    expect(merged.mappedModel).toBe('mapped-live');
    expect(merged.responseModel).toBe('response-live');
    expect(merged.inputTokenCount).toBe(101);
    expect(merged.outputTokenCount).toBe(202);
    expect(merged.cacheReadCount).toBe(303);
    expect(merged.cacheWriteCount).toBe(404);
    expect(merged.cost).toBe(999);
  });

  it('preserves zero values from live attempt summaries', () => {
    const original = {
      ...request('IN_PROGRESS'),
      routeID: 7,
      providerID: 9,
      ttft: 123,
      inputTokenCount: 101,
      outputTokenCount: 202,
      cacheReadCount: 303,
      cacheWriteCount: 404,
      cache5mWriteCount: 50,
      cache1hWriteCount: 60,
      modelPriceId: 12,
      multiplier: 15000,
      cost: 999,
    };

    const merged = mergeProxyRequestAttemptUpdate(original, {
      ...attempt,
      routeID: 0,
      providerID: 0,
      ttft: 0,
      inputTokenCount: 0,
      outputTokenCount: 0,
      cacheReadCount: 0,
      cacheWriteCount: 0,
      cache5mWriteCount: 0,
      cache1hWriteCount: 0,
      modelPriceId: 0,
      multiplier: 0,
      cost: 0,
    });

    expect(merged.routeID).toBe(0);
    expect(merged.providerID).toBe(0);
    expect(merged.ttft).toBe(0);
    expect(merged.inputTokenCount).toBe(0);
    expect(merged.outputTokenCount).toBe(0);
    expect(merged.cacheReadCount).toBe(0);
    expect(merged.cacheWriteCount).toBe(0);
    expect(merged.cache5mWriteCount).toBe(0);
    expect(merged.cache1hWriteCount).toBe(0);
    expect(merged.modelPriceId).toBe(0);
    expect(merged.multiplier).toBe(0);
    expect(merged.cost).toBe(0);
  });

  it('ignores updates for another request', () => {
    const original = request('IN_PROGRESS');
    const merged = mergeProxyRequestAttemptUpdate(original, { ...attempt, proxyRequestID: 2 });

    expect(merged).toBe(original);
  });
});
