import { describe, expect, it } from 'vitest';
import type { ProxyRequest } from '@/lib/transport';
import { isProxyRequestError } from './use-requests';

const baseRequest = {
  id: 1,
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
  instanceID: 'instance-1',
  requestID: 'request-1',
  sessionID: 'session-1',
  clientType: 'openai',
  requestModel: 'model-a',
  responseModel: '',
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
