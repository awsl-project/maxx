import { describe, expect, it } from 'vitest';
import { invertVisibleProviderSelection } from './selection';

describe('provider selection helpers', () => {
  it('inverts only visible provider IDs and preserves hidden selections', () => {
    const selected = new Set([1, 3, 99]);

    const result = invertVisibleProviderSelection(selected, [1, 2, 3, 4]);

    expect([...result].sort((a, b) => a - b)).toEqual([2, 4, 99]);
    expect([...selected].sort((a, b) => a - b)).toEqual([1, 3, 99]);
  });

  it('does not clear hidden selections when no providers are visible', () => {
    const result = invertVisibleProviderSelection(new Set([42]), []);

    expect([...result]).toEqual([42]);
  });
});
