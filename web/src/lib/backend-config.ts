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

// Must match AUTH_TOKEN_KEY in lib/auth-context.ts. Duplicated here (rather than
// imported) to avoid an import cycle: auth-context → transport → backend-config.
const AUTH_TOKEN_KEY = 'maxx-admin-token';

/**
 * Thrown when persisting the backend URL fails because storage is unavailable
 * (e.g. private mode, locked-down environments) — as opposed to the URL being
 * invalid. Lets the UI surface a distinct, accurate error.
 */
export class BackendStorageError extends Error {
  constructor(cause?: unknown) {
    super('Failed to persist backend URL: storage unavailable');
    this.name = 'BackendStorageError';
    if (cause !== undefined) {
      (this as { cause?: unknown }).cause = cause;
    }
  }
}

/** Build-time fallback (empty string when unset). Read lazily so it can be tested. */
function buildTimeBackendUrl(): string {
  return (import.meta.env.VITE_BACKEND_URL as string | undefined)?.trim() ?? '';
}

/**
 * Normalizes a backend URL to `origin + pathname` (query and hash dropped,
 * trailing slashes stripped) so that later appending "/api" or "/ws" never
 * yields a malformed URL. A sub-path is preserved to support a backend behind a
 * reverse-proxy prefix (e.g. https://example.com/maxx). Empty input → "".
 *
 * @throws Error if the (non-empty) input is not a valid absolute http(s) URL.
 */
function normalizeBackendUrl(raw: string): string {
  const trimmed = raw.trim();
  if (!trimmed) {
    return '';
  }
  const parsed = new URL(trimmed); // throws on an unparseable URL
  if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
    throw new Error('Backend URL must use http or https');
  }
  return (parsed.origin + parsed.pathname).replace(/\/+$/, '');
}

/** Like normalizeBackendUrl but returns "" instead of throwing on bad input. */
function normalizeBackendUrlSafe(raw: string): string {
  try {
    return normalizeBackendUrl(raw);
  } catch {
    return '';
  }
}

/**
 * Returns the configured backend base (origin, plus any reverse-proxy sub-path),
 * or an empty string when the UI should use its own origin (same-origin default).
 * The runtime override and the build-time VITE_BACKEND_URL fallback share the
 * same normalization, so both honor the identical URL contract.
 */
export function getBackendUrl(): string {
  let stored = '';
  try {
    stored = localStorage.getItem(STORAGE_KEY)?.trim() ?? '';
  } catch {
    // localStorage may be unavailable (private mode / SSR); fall through.
  }
  return normalizeBackendUrlSafe(stored || buildTimeBackendUrl());
}

/**
 * Persists a backend URL override. Pass an empty/whitespace string to clear it
 * and revert to the build-time / same-origin default. Returns the normalized
 * effective backend after the change.
 *
 * The input is normalized to `origin + pathname` (query and hash are dropped,
 * trailing slashes stripped) so that later appending "/api" never yields a
 * malformed URL. A sub-path is preserved to support a backend hosted behind a
 * reverse-proxy prefix (e.g. https://example.com/maxx).
 *
 * When the effective backend actually changes, the stored admin token is
 * cleared so a session minted by one backend is never replayed against another.
 *
 * @throws Error if the provided value is not a valid absolute http(s) URL.
 */
export function setBackendUrl(raw: string): string {
  const previous = getBackendUrl();

  // Validate/normalize before touching storage so a bad URL never half-applies.
  // Shared with getBackendUrl() so runtime and build-time values are identical.
  const normalized = normalizeBackendUrl(raw); // throws on an invalid URL; "" for empty

  // Storage failures (private mode, locked-down env) are a distinct error from
  // an invalid URL, so the UI can report them accurately rather than as "invalid".
  try {
    if (!normalized) {
      localStorage.removeItem(STORAGE_KEY);
    } else {
      localStorage.setItem(STORAGE_KEY, normalized);
    }
  } catch (err) {
    throw new BackendStorageError(err);
  }

  const current = getBackendUrl();
  if (current !== previous) {
    // Don't carry one backend's credentials over to another origin.
    try {
      localStorage.removeItem(AUTH_TOKEN_KEY);
    } catch {
      // ignore storage errors
    }
  }
  return current;
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
