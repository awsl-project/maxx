/**
 * Backend address configuration.
 *
 * By default the web UI talks to the same origin that served it (baseURL
 * `/api`, WebSocket on `/ws`). This module lets a separately-hosted frontend
 * (e.g. a static build on a CDN, or a dev server) point at an arbitrary backend
 * by storing an override in localStorage. A build-time `VITE_BACKEND_URL` is
 * used as the fallback when no runtime override is present.
 *
 * The backend must allow the frontend's origin via MAXX_CORS_ALLOW_ORIGINS for
 * cross-origin requests to succeed.
 */

import type { TransportConfig } from './transport/interface';

const STORAGE_KEY = 'maxx_backend_url';

/** Build-time fallback (empty string when unset). */
const BUILD_TIME_BACKEND_URL: string =
  (import.meta.env.VITE_BACKEND_URL as string | undefined)?.trim() ?? '';

/**
 * Returns the configured backend base origin (no trailing slash), or an empty
 * string when the UI should use its own origin (same-origin default).
 */
export function getBackendUrl(): string {
  let stored = '';
  try {
    stored = localStorage.getItem(STORAGE_KEY)?.trim() ?? '';
  } catch {
    // localStorage may be unavailable (private mode / SSR); fall through.
  }
  const value = stored || BUILD_TIME_BACKEND_URL;
  return value.replace(/\/+$/, '');
}

/**
 * Persists a backend URL override. Pass an empty/whitespace string to clear it
 * and revert to the same-origin default. Returns the normalized value stored.
 *
 * @throws Error if the provided value is not a valid absolute http(s) URL.
 */
export function setBackendUrl(raw: string): string {
  const trimmed = raw.trim().replace(/\/+$/, '');
  if (!trimmed) {
    localStorage.removeItem(STORAGE_KEY);
    return '';
  }
  // Validate: must be an absolute http(s) URL.
  const parsed = new URL(trimmed);
  if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
    throw new Error('Backend URL must use http or https');
  }
  localStorage.setItem(STORAGE_KEY, trimmed);
  return trimmed;
}

/**
 * Builds the TransportConfig (baseURL / adminBaseURL / wsURL) for the configured
 * backend. Returns `undefined` when no override is set, so HttpTransport applies
 * its same-origin defaults.
 */
export function buildTransportConfig(): TransportConfig | undefined {
  const backend = getBackendUrl();
  if (!backend) {
    return undefined;
  }

  const baseURL = `${backend}/api`;
  // Derive ws(s):// from the backend origin.
  const wsOrigin = backend.replace(/^http/, 'ws');
  const wsURL = `${wsOrigin}/ws`;

  return {
    baseURL,
    adminBaseURL: `${baseURL}/admin`,
    wsURL,
  };
}
