import { describe, expect, it } from 'vitest';
import { normalizeMaxConcurrency } from './provider-max-concurrency-field';

describe('normalizeMaxConcurrency', () => {
  it('treats empty/nullish as unlimited (0)', () => {
    expect(normalizeMaxConcurrency('')).toBe(0);
    expect(normalizeMaxConcurrency(null)).toBe(0);
    expect(normalizeMaxConcurrency(undefined)).toBe(0);
  });

  it('truncates positives to non-negative integers', () => {
    expect(normalizeMaxConcurrency(3.9)).toBe(3);
    expect(normalizeMaxConcurrency('12')).toBe(12);
    expect(normalizeMaxConcurrency(' 7 ')).toBe(7);
    expect(normalizeMaxConcurrency(0)).toBe(0);
  });

  it('clamps invalid and negative values to 0', () => {
    expect(normalizeMaxConcurrency(-1)).toBe(0);
    expect(normalizeMaxConcurrency('-3')).toBe(0);
    expect(normalizeMaxConcurrency('abc')).toBe(0);
    expect(normalizeMaxConcurrency(Number.NaN)).toBe(0);
    expect(normalizeMaxConcurrency(Number.POSITIVE_INFINITY)).toBe(0);
  });
});
