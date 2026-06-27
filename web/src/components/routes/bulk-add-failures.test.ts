import { describe, expect, it } from 'vitest';
import { createBulkAddRouteFailure, getBulkAddRouteFailureMessage } from './bulk-add-failures';
import type { Provider } from '@/lib/transport';

describe('bulk add route failures', () => {
  it('uses backend response error fields before generic error messages', () => {
    expect(
      getBulkAddRouteFailureMessage({
        message: 'Request failed with status code 409',
        response: { data: { error: 'provider already has a claude route' } },
      }),
    ).toBe('provider already has a claude route');
  });

  it('uses plain response body text when available', () => {
    expect(
      getBulkAddRouteFailureMessage({
        response: { data: 'database is locked' },
      }),
    ).toBe('database is locked');
  });

  it('falls back to the thrown error message', () => {
    expect(getBulkAddRouteFailureMessage(new Error('network timeout'))).toBe('network timeout');
  });

  it('builds provider-scoped failure details for the UI', () => {
    const provider = {
      id: 41,
      name: 'Claude Bad Gateway',
      type: 'custom',
      config: null,
      supportedClientTypes: ['claude'],
      createdAt: '2026-06-27T00:00:00Z',
      updatedAt: '2026-06-27T00:00:00Z',
    } satisfies Provider;

    expect(
      createBulkAddRouteFailure(provider, {
        response: { data: { message: 'upstream rejected the route' } },
      }),
    ).toEqual({
      providerID: 41,
      providerName: 'Claude Bad Gateway',
      providerType: 'custom',
      message: 'upstream rejected the route',
    });
  });
});
