import { describe, expect, it } from 'vitest';

import { API_TOKEN_INACTIVE_EXPIRY_MS, isAPITokenExpired } from './api-token-expiry';

describe('isAPITokenExpired', () => {
  const now = new Date('2026-06-17T12:00:00Z');

  it('expires a token last used fifteen days ago', () => {
    expect(
      isAPITokenExpired(
        { lastUsedAt: new Date(now.getTime() - 15 * 24 * 60 * 60 * 1000).toISOString() },
        now,
      ),
    ).toBe(true);
  });

  it('does not apply inactivity expiry when lastUsedAt is missing', () => {
    expect(isAPITokenExpired({}, now)).toBe(false);
  });

  it('keeps the exact ten-day boundary valid', () => {
    expect(
      isAPITokenExpired(
        { lastUsedAt: new Date(now.getTime() - API_TOKEN_INACTIVE_EXPIRY_MS).toISOString() },
        now,
      ),
    ).toBe(false);
  });

  it('still honors explicit expiresAt', () => {
    expect(isAPITokenExpired({ expiresAt: '2026-06-16T12:00:00Z' }, now)).toBe(true);
  });
});
