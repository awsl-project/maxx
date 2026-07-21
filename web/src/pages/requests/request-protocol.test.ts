import { describe, expect, it } from 'vitest';
import { resolveRequestProtocol } from './request-protocol';

describe('resolveRequestProtocol', () => {
  it('prefers stored protocol', () => {
    expect(
      resolveRequestProtocol({ protocol: 'websocket', isStream: true, statusCode: 200 }),
    ).toBe('websocket');
    expect(resolveRequestProtocol({ protocol: 'SSE', isStream: false, statusCode: 0 })).toBe(
      'sse',
    );
  });

  it('falls back for historical rows', () => {
    expect(resolveRequestProtocol({ protocol: '', isStream: true, statusCode: 101 })).toBe(
      'websocket',
    );
    expect(resolveRequestProtocol({ protocol: '', isStream: true, statusCode: 200 })).toBe('sse');
    expect(resolveRequestProtocol({ protocol: '', isStream: false, statusCode: 200 })).toBe(
      'http',
    );
  });
});
