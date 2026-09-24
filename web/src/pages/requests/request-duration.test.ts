import { describe, expect, it } from 'vitest';
import type { ProxyRequest } from '@/lib/transport/types';
import {
  formatDurationMs,
  formatDurationNs,
  formatRequestDuration,
  getActiveRequestDurationMs,
  getRequestStartTimestampMs,
  isActiveProxyRequest,
} from './request-duration';

const createdAt = '2026-09-24T20:00:00.000Z';
const startTime = '2026-09-24T20:00:05.000Z';
const nowMs = new Date('2026-09-24T20:00:08.500Z').getTime();

type RequestDurationFixture = Pick<ProxyRequest, 'status' | 'startTime' | 'createdAt' | 'duration'>;

function request(overrides: Partial<RequestDurationFixture> = {}): RequestDurationFixture {
  return {
    status: 'COMPLETED',
    startTime,
    createdAt,
    duration: 1_250_000_000,
    ...overrides,
  };
}

describe('request duration display helpers', () => {
  it('treats PENDING and IN_PROGRESS as active requests', () => {
    expect(isActiveProxyRequest(request({ status: 'PENDING' }))).toBe(true);
    expect(isActiveProxyRequest(request({ status: 'IN_PROGRESS' }))).toBe(true);
    expect(isActiveProxyRequest(request({ status: 'COMPLETED' }))).toBe(false);
    expect(isActiveProxyRequest(request({ status: 'FAILED' }))).toBe(false);
  });

  it('formats stored terminal nanosecond durations', () => {
    expect(formatDurationNs(1_250_000_000)).toBe('1.25s');
    expect(formatDurationNs(0)).toBe('-');
    expect(formatDurationNs(null)).toBe('-');
  });

  it('formats live active millisecond durations including zero', () => {
    expect(formatDurationMs(3500)).toBe('3.50s');
    expect(formatDurationMs(0)).toBe('0.00s');
    expect(formatDurationMs(null)).toBe('-');
  });

  it('uses startTime before createdAt for live active duration', () => {
    expect(getRequestStartTimestampMs(request())).toBe(new Date(startTime).getTime());
    expect(getActiveRequestDurationMs(request({ status: 'IN_PROGRESS' }), nowMs)).toBe(3500);
    expect(formatRequestDuration(request({ status: 'IN_PROGRESS', duration: 0 }), nowMs)).toBe(
      '3.50s',
    );
  });

  it('falls back to createdAt when startTime is missing or a Go zero time', () => {
    expect(
      getRequestStartTimestampMs(request({ startTime: '0001-01-01T00:00:00Z', createdAt })),
    ).toBe(new Date(createdAt).getTime());
    expect(
      formatRequestDuration(
        request({ status: 'PENDING', startTime: '0001-01-01T00:00:00Z', duration: 0 }),
        nowMs,
      ),
    ).toBe('8.50s');
  });

  it('never shows a negative active duration when the client clock is behind', () => {
    expect(
      formatRequestDuration(
        request({ status: 'IN_PROGRESS', startTime: '2026-09-24T20:01:00.000Z', duration: 0 }),
        nowMs,
      ),
    ).toBe('0.00s');
  });

  it('keeps terminal rows on persisted duration instead of wall-clock time', () => {
    expect(formatRequestDuration(request({ status: 'COMPLETED' }), nowMs)).toBe('1.25s');
  });
});
