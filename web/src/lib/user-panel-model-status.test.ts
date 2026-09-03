import { describe, expect, it } from 'vitest';
import type { ProxyRequest } from '@/lib/transport';
import { buildUserPanelModelStatuses } from './user-panel-model-status';

function request(overrides: Partial<ProxyRequest>): ProxyRequest {
  return {
    id: 1,
    createdAt: '2026-09-03T00:00:00Z',
    updatedAt: '2026-09-03T00:00:00Z',
    instanceID: '',
    requestID: '',
    sessionID: '',
    clientType: 'openai',
    requestModel: 'gpt-5',
    mappedModel: '',
    responseModel: '',
    reasoningEffort: '',
    startTime: '',
    endTime: '',
    duration: 0,
    ttft: 0,
    isStream: false,
    status: 'COMPLETED',
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
    multiplier: 0,
    cost: 0,
    apiTokenID: 0,
    ...overrides,
  };
}

describe('buildUserPanelModelStatuses', () => {
  it('marks available models without recent requests as no data', () => {
    expect(
      buildUserPanelModelStatuses({
        availableModels: ['gemini-2.5-pro'],
        requests: [],
        now: Date.parse('2026-09-03T00:00:00Z'),
      }),
    ).toEqual([
      {
        model: 'gemini-2.5-pro',
        health: 'no-data',
        totalRequests: 0,
        successfulRequests: 0,
        failedRequests: 0,
        consecutiveFailures: 0,
      },
    ]);
  });

  it('marks successful recent models as healthy', () => {
    const statuses = buildUserPanelModelStatuses({
      availableModels: ['gpt-5'],
      requests: [request({ requestModel: 'gpt-5', status: 'COMPLETED', statusCode: 200 })],
      now: Date.parse('2026-09-03T00:00:00Z'),
    });

    expect(statuses[0]).toMatchObject({
      model: 'gpt-5',
      health: 'healthy',
      totalRequests: 1,
      successfulRequests: 1,
      failedRequests: 0,
    });
  });

  it('marks models with some recent failures as degraded', () => {
    const statuses = buildUserPanelModelStatuses({
      availableModels: ['claude-sonnet-4'],
      requests: [
        request({
          id: 2,
          requestModel: 'claude-sonnet-4',
          createdAt: '2026-09-03T00:00:00Z',
          status: 'COMPLETED',
          statusCode: 200,
        }),
        request({
          id: 1,
          requestModel: 'claude-sonnet-4',
          createdAt: '2026-09-02T23:00:00Z',
          status: 'FAILED',
          statusCode: 502,
          error: 'upstream timeout',
        }),
      ],
      now: Date.parse('2026-09-03T00:00:00Z'),
    });

    expect(statuses[0]).toMatchObject({
      model: 'claude-sonnet-4',
      health: 'degraded',
      totalRequests: 2,
      successfulRequests: 1,
      failedRequests: 1,
      consecutiveFailures: 0,
      lastError: 'upstream timeout',
    });
  });

  it('marks models with three latest failures as unavailable', () => {
    const statuses = buildUserPanelModelStatuses({
      availableModels: ['bad-model'],
      requests: [
        request({
          id: 3,
          requestModel: 'bad-model',
          createdAt: '2026-09-03T00:00:00Z',
          status: 'FAILED',
          statusCode: 502,
          error: 'bad gateway',
        }),
        request({
          id: 2,
          requestModel: 'bad-model',
          createdAt: '2026-09-02T23:59:00Z',
          status: 'REJECTED',
          statusCode: 429,
          error: 'rate limited',
        }),
        request({
          id: 1,
          requestModel: 'bad-model',
          createdAt: '2026-09-02T23:58:00Z',
          status: 'COMPLETED',
          statusCode: 500,
          error: 'server error',
        }),
      ],
      now: Date.parse('2026-09-03T00:00:00Z'),
    });

    expect(statuses[0]).toMatchObject({
      model: 'bad-model',
      health: 'unavailable',
      failedRequests: 3,
      consecutiveFailures: 3,
      lastError: 'bad gateway',
    });
  });

  it('ignores requests outside the last 24 hours', () => {
    const statuses = buildUserPanelModelStatuses({
      availableModels: ['gpt-5'],
      requests: [
        request({
          requestModel: 'gpt-5',
          createdAt: '2026-09-01T23:59:59Z',
          status: 'FAILED',
          statusCode: 502,
        }),
      ],
      now: Date.parse('2026-09-03T00:00:00Z'),
    });

    expect(statuses[0]).toMatchObject({ health: 'no-data', totalRequests: 0 });
  });
});
