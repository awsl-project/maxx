import { describe, expect, it } from 'vitest';
import {
  getUserPanelAnnouncementFingerprint,
  getUserPanelAnnouncementSeenStorageKey,
  isUserPanelAnnouncementUnread,
} from './user-panel-announcement';

describe('user-panel-announcement', () => {
  it('scopes seen storage by user id', () => {
    expect(getUserPanelAnnouncementSeenStorageKey(42)).toBe('maxx:user-panel:announcement-seen:42');
    expect(getUserPanelAnnouncementSeenStorageKey('member')).toBe(
      'maxx:user-panel:announcement-seen:member',
    );
    expect(getUserPanelAnnouncementSeenStorageKey(null)).toBe(
      'maxx:user-panel:announcement-seen:anonymous',
    );
  });

  it('marks non-empty unseen announcements as unread', () => {
    expect(isUserPanelAnnouncementUnread('hello', null)).toBe(true);
    expect(isUserPanelAnnouncementUnread('   ', null)).toBe(false);
  });

  it('clears unread state only for the matching content fingerprint', () => {
    const oldFingerprint = getUserPanelAnnouncementFingerprint('old notice');
    const newFingerprint = getUserPanelAnnouncementFingerprint('new notice');

    expect(isUserPanelAnnouncementUnread('old notice', oldFingerprint)).toBe(false);
    expect(isUserPanelAnnouncementUnread('new notice', oldFingerprint)).toBe(true);
    expect(newFingerprint).not.toBe(oldFingerprint);
  });
});
