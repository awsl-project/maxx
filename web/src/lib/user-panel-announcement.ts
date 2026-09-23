const ANNOUNCEMENT_SEEN_STORAGE_PREFIX = 'maxx:user-panel:announcement-seen';

export function getUserPanelAnnouncementSeenStorageKey(
  userId: number | string | null | undefined,
): string {
  const scope =
    userId === null || userId === undefined || userId === '' ? 'anonymous' : String(userId);
  return `${ANNOUNCEMENT_SEEN_STORAGE_PREFIX}:${scope}`;
}

export function getUserPanelAnnouncementFingerprint(markdown: string): string {
  const normalized = markdown.trim();
  if (!normalized) return '';

  let hash = 0x811c9dc5;
  for (let i = 0; i < normalized.length; i += 1) {
    hash ^= normalized.charCodeAt(i);
    hash = Math.imul(hash, 0x01000193);
  }
  return `fnv1a:${(hash >>> 0).toString(16).padStart(8, '0')}`;
}

export function isUserPanelAnnouncementUnread(
  markdown: string,
  seenFingerprint: string | null | undefined,
): boolean {
  const fingerprint = getUserPanelAnnouncementFingerprint(markdown);
  return fingerprint !== '' && seenFingerprint !== fingerprint;
}
