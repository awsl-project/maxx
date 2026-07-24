import { describe, expect, it } from 'vitest';
import { formatAPITokenPrefix } from './token-display';

describe('formatAPITokenPrefix', () => {
  it('compresses generated maxx prefixes to a stable short display', () => {
    expect(formatAPITokenPrefix('maxx76dd169738b9f4269a4...')).toBe('maxx76dd…69a4');
    expect(formatAPITokenPrefix('maxx_76dd169738b9f4269a4...')).toBe('maxx_76dd…69a4');
  });

  it('keeps short historical prefixes readable', () => {
    expect(formatAPITokenPrefix('maxxexp')).toBe('maxxexp');
    expect(formatAPITokenPrefix('maxxtest')).toBe('maxxtest');
  });

  it('falls back safely for empty and non-maxx prefixes', () => {
    expect(formatAPITokenPrefix('')).toBe('—');
    expect(formatAPITokenPrefix('legacy-token-prefix-123456')).toBe('legacy…3456');
  });
});
