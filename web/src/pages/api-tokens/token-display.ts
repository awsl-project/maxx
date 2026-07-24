const MAXX_TOKEN_PREFIX_PATTERN = /^(maxx_?)(.*)$/i;

export function formatAPITokenPrefix(tokenPrefix?: string): string {
  const trimmed = tokenPrefix?.trim() ?? '';
  if (!trimmed) {
    return '—';
  }

  const withoutTrailingEllipsis = trimmed.replace(/\.{3,}$/, '');
  const match = withoutTrailingEllipsis.match(MAXX_TOKEN_PREFIX_PATTERN);
  if (!match) {
    return withoutTrailingEllipsis.length > 12
      ? `${withoutTrailingEllipsis.slice(0, 6)}…${withoutTrailingEllipsis.slice(-4)}`
      : withoutTrailingEllipsis;
  }

  const [, marker, body] = match;
  if (body.length <= 8) {
    return withoutTrailingEllipsis;
  }

  return `${marker}${body.slice(0, 4)}…${body.slice(-4)}`;
}
