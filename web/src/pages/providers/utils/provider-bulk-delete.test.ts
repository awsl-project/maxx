import { describe, expect, it } from 'vitest';
import { buildProviderBulkDeleteStatus } from './provider-bulk-delete';
import type { Provider } from '@/lib/transport';

function provider(id: number, name: string): Provider {
  return {
    id,
    tenantID: 1,
    type: 'custom',
    name,
    logo: '',
    config: {},
    supportedClientTypes: [],
    supportModels: [],
    excludeFromExport: false,
    createdAt: '2026-06-27T00:00:00Z',
    updatedAt: '2026-06-27T00:00:00Z',
  };
}

describe('provider bulk delete result helpers', () => {
  it('treats not-found providers as resolved instead of failed', () => {
    const status = buildProviderBulkDeleteStatus(
      [provider(101, 'expired-a'), provider(102, 'expired-b')],
      {
        deletedCount: 1,
        deletedIDs: [101],
        notFoundIDs: [102],
        routeDeletedCount: 2,
        modelMappingDeletedCount: 1,
      },
      'Provider was not deleted',
    );

    expect(status).toEqual({ deleted: 2, failed: [] });
  });

  it('keeps selected providers failed when the backend neither deleted nor reported them missing', () => {
    const status = buildProviderBulkDeleteStatus(
      [provider(101, 'expired-a'), provider(103, 'still-present')],
      {
        deletedCount: 1,
        deletedIDs: [101],
        notFoundIDs: [],
        routeDeletedCount: 1,
        modelMappingDeletedCount: 0,
      },
      'Provider was not deleted',
    );

    expect(status).toEqual({
      deleted: 1,
      failed: [{ id: 103, name: 'still-present', message: 'Provider was not deleted' }],
    });
  });
});
