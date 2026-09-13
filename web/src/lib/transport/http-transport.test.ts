import { describe, expect, it } from 'vitest';
import { serializeQueryParams } from './http-transport';

describe('serializeQueryParams', () => {
  it('serializes array values as repeated query keys for backend filters', () => {
    expect(
      serializeQueryParams({
        status: 'FAILED',
        errorContains: ['failed to connect', 'context canceled'],
        empty: undefined,
      }),
    ).toBe('status=FAILED&errorContains=failed+to+connect&errorContains=context+canceled');
  });
});
