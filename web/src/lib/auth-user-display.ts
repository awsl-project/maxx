import type { AuthUser } from './auth-context';

function compactWhitespace(value?: string | null): string {
  return value?.trim().replace(/\s+/g, ' ') ?? '';
}

function maskSegment(value: string): string {
  const chars = Array.from(value);
  if (chars.length <= 1) {
    return value;
  }
  if (chars.length === 2) {
    return `${chars[0]}*`;
  }
  if (chars.length === 3) {
    return `${chars[0]}*${chars[2]}`;
  }
  return `${chars[0]}${'*'.repeat(Math.min(3, chars.length - 2))}${chars[chars.length - 1]}`;
}

export function maskAccountIdentifier(value?: string | null): string {
  const normalized = compactWhitespace(value);
  if (!normalized) {
    return '';
  }

  if (/^(?:UID-\d+|T-\d+)$/i.test(normalized)) {
    return normalized;
  }

  if (normalized.includes('@')) {
    const [localPart, domainPart = ''] = normalized.split('@');
    const domainSegments = domainPart.split('.');
    const domainName = domainSegments.shift() ?? '';
    const suffix = domainSegments.length > 0 ? `.${domainSegments.join('.')}` : '';
    return `${maskSegment(localPart)}@${maskSegment(domainName)}${suffix}`;
  }

  return maskSegment(normalized);
}

export interface AuthUserDisplay {
  maskedIdentity: string;
  rawIdentity: string;
  tenantLabel: string;
  userLabel: string;
  tenantIDLabel: string;
  avatarFallback: string;
}

export function getAuthUserDisplay(user?: AuthUser | null): AuthUserDisplay {
  const username = compactWhitespace(user?.username);
  const tenantName = compactWhitespace(user?.tenantName);
  const hasUsername = username !== '';
  const hasTenantName = tenantName !== '';
  const userId = user?.id;
  const tenantId = user?.tenantID;
  const hasUserId = userId != null;
  const hasTenantId = tenantId != null;
  const rawIdentity = hasUsername
    ? username
    : hasUserId
      ? `UID-${userId}`
      : hasTenantId
        ? `T-${tenantId}`
        : 'Maxx';
  const avatarSource = hasUsername
    ? username
    : hasUserId
      ? `${userId}`
      : hasTenantName
        ? tenantName
        : 'MX';
  const avatarFallback = (Array.from(avatarSource.replace(/\s+/g, ''))
    .slice(0, 2)
    .join('') || 'MX'
  ).toUpperCase();

  return {
    maskedIdentity: maskAccountIdentifier(rawIdentity) || 'Maxx',
    rawIdentity,
    tenantLabel: hasTenantName
      ? maskAccountIdentifier(tenantName)
      : hasTenantId
        ? `T-${tenantId}`
        : 'T-?',
    userLabel: hasUserId ? `UID-${userId}` : 'UID-?',
    tenantIDLabel: hasTenantId ? `T-${tenantId}` : 'T-?',
    avatarFallback,
  };
}
