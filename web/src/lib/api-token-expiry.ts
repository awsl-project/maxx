import type { APIToken } from '@/lib/transport';

export const API_TOKEN_INACTIVE_EXPIRY_MS = 10 * 24 * 60 * 60 * 1000;

export function isAPITokenExpired(token: Pick<APIToken, 'expiresAt' | 'lastUsedAt'>, now = new Date()): boolean {
  if (token.expiresAt && new Date(token.expiresAt) < now) {
    return true;
  }
  if (!token.lastUsedAt) {
    return false;
  }
  return now.getTime() > new Date(token.lastUsedAt).getTime() + API_TOKEN_INACTIVE_EXPIRY_MS;
}
