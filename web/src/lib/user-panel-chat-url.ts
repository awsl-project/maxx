export function buildUserPanelChatUrl(origin: string, token: string): string {
  const cleanOrigin = origin.replace(/\/$/, '');
  const cleanToken = token.trim();
  if (!cleanOrigin || !cleanToken) return '';
  return `${cleanOrigin}/chat#${encodeURIComponent(cleanToken)}`;
}
