import { describe, expect, it } from 'vitest';
import { getAntigravityAvailabilityInfo } from './antigravity-availability';
import type { AntigravityQuotaData } from '@/lib/transport';

const quota = (percentage: number): AntigravityQuotaData => ({
  models: [{ name: 'claude-sonnet-4', percentage, resetTime: '2099-01-01T00:00:00Z' }],
  lastUpdated: 1,
  isForbidden: false,
  subscriptionTier: 'PRO',
});

describe('getAntigravityAvailabilityInfo', () => {
  it('treats explicit forbidden quota as unavailable', () => {
    expect(getAntigravityAvailabilityInfo({ ...quota(80), isForbidden: true }).status).toBe(
      'forbidden',
    );
  });

  it('treats zero Claude quota as exhausted', () => {
    expect(getAntigravityAvailabilityInfo(quota(0)).status).toBe('exhausted');
  });

  it('treats low Claude quota as warning', () => {
    expect(getAntigravityAvailabilityInfo(quota(20)).status).toBe('low');
  });

  it('does not mark missing quota as unavailable', () => {
    expect(getAntigravityAvailabilityInfo(undefined).status).toBe('unknown');
  });

  it('prefers cooldown over quota availability', () => {
    expect(getAntigravityAvailabilityInfo(quota(80), 'limited').status).toBe('cooldown');
  });
});
