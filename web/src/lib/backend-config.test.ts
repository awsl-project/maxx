import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  BackendStorageError,
  buildTransportConfig,
  getBackendUrl,
  setBackendUrl,
} from './backend-config';

const BACKEND_KEY = 'maxx_backend_url';
const AUTH_KEY = 'maxx-admin-token';

beforeEach(() => {
  localStorage.clear();
});

describe('getBackendUrl', () => {
  it('returns empty string when nothing is configured (same-origin default)', () => {
    expect(getBackendUrl()).toBe('');
  });

  it('returns the stored override without a trailing slash', () => {
    localStorage.setItem(BACKEND_KEY, 'https://api.example.com');
    expect(getBackendUrl()).toBe('https://api.example.com');
  });
});

describe('setBackendUrl normalization', () => {
  it('stores a plain origin as-is', () => {
    expect(setBackendUrl('https://api.example.com')).toBe('https://api.example.com');
    expect(localStorage.getItem(BACKEND_KEY)).toBe('https://api.example.com');
  });

  it('strips trailing slashes', () => {
    expect(setBackendUrl('https://api.example.com///')).toBe('https://api.example.com');
  });

  it('drops query and hash so appending /api stays valid', () => {
    expect(setBackendUrl('https://api.example.com?x=1#frag')).toBe('https://api.example.com');
  });

  it('preserves a reverse-proxy sub-path', () => {
    expect(setBackendUrl('https://example.com/maxx/')).toBe('https://example.com/maxx');
  });

  it('rejects non-http(s) URLs', () => {
    expect(() => setBackendUrl('ftp://api.example.com')).toThrow();
    expect(() => setBackendUrl('not a url')).toThrow();
  });

  it('throws a distinct BackendStorageError when storage is unavailable', () => {
    const spy = vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('storage denied');
    });
    try {
      expect(() => setBackendUrl('https://api.example.com')).toThrow(BackendStorageError);
      // A genuinely invalid URL still throws the plain validation error, not storage.
      expect(() => setBackendUrl('not a url')).not.toThrow(BackendStorageError);
    } finally {
      spy.mockRestore();
    }
  });

  it('clearing reverts to same-origin default', () => {
    setBackendUrl('https://api.example.com');
    expect(setBackendUrl('   ')).toBe('');
    expect(localStorage.getItem(BACKEND_KEY)).toBeNull();
  });
});

describe('setBackendUrl auth-token handling', () => {
  it('clears the stored admin token when the backend changes', () => {
    localStorage.setItem(AUTH_KEY, 'jwt-for-backend-a');
    setBackendUrl('https://api.example.com');
    expect(localStorage.getItem(AUTH_KEY)).toBeNull();
  });

  it('keeps the token when the effective backend is unchanged', () => {
    setBackendUrl('https://api.example.com');
    localStorage.setItem(AUTH_KEY, 'jwt');
    // Re-saving the same (normalized) URL must not log the user out.
    setBackendUrl('https://api.example.com/');
    expect(localStorage.getItem(AUTH_KEY)).toBe('jwt');
  });

  it('clears the token when reverting to same-origin', () => {
    setBackendUrl('https://api.example.com');
    localStorage.setItem(AUTH_KEY, 'jwt');
    setBackendUrl('');
    expect(localStorage.getItem(AUTH_KEY)).toBeNull();
  });
});

describe('buildTransportConfig', () => {
  it('returns undefined for the same-origin default', () => {
    expect(buildTransportConfig()).toBeUndefined();
  });

  it('derives base/admin/ws URLs from an https backend', () => {
    setBackendUrl('https://api.example.com');
    expect(buildTransportConfig()).toEqual({
      baseURL: 'https://api.example.com/api',
      adminBaseURL: 'https://api.example.com/api/admin',
      wsURL: 'wss://api.example.com/ws',
    });
  });

  it('derives a ws:// URL from an http backend', () => {
    setBackendUrl('http://localhost:9880');
    expect(buildTransportConfig()?.wsURL).toBe('ws://localhost:9880/ws');
  });

  it('normalizes the build-time VITE_BACKEND_URL through the same contract', () => {
    // No runtime override; the build-time fallback carries query/hash + trailing slash.
    vi.stubEnv('VITE_BACKEND_URL', 'https://api.example.com/?x=1#frag');
    try {
      expect(getBackendUrl()).toBe('https://api.example.com');
      expect(buildTransportConfig()).toEqual({
        baseURL: 'https://api.example.com/api',
        adminBaseURL: 'https://api.example.com/api/admin',
        wsURL: 'wss://api.example.com/ws',
      });
    } finally {
      vi.unstubAllEnvs();
    }
  });
});
