import { describe, expect, it } from 'vitest';
import {
  getUserPanelTabStorageKey,
  isUserPanelTab,
  resolveUserPanelTab,
  updateUserPanelTabSearch,
} from './user-panel-tabs';

describe('user-panel-tabs', () => {
  it('validates only supported user panel tabs', () => {
    expect(isUserPanelTab('main')).toBe(true);
    expect(isUserPanelTab('requests')).toBe(true);
    expect(isUserPanelTab('model-status')).toBe(true);
    expect(isUserPanelTab('usage')).toBe(false);
    expect(isUserPanelTab('')).toBe(false);
    expect(isUserPanelTab(null)).toBe(false);
  });

  it('scopes stored tab keys by account id', () => {
    expect(getUserPanelTabStorageKey(42)).toBe('maxx:user-panel:active-tab:42');
    expect(getUserPanelTabStorageKey('member')).toBe('maxx:user-panel:active-tab:member');
    expect(getUserPanelTabStorageKey(null)).toBe('maxx:user-panel:active-tab:anonymous');
  });

  it('prefers a valid URL tab over a stored tab', () => {
    expect(resolveUserPanelTab({ urlTab: 'requests', storedTab: 'main' })).toBe('requests');
    expect(resolveUserPanelTab({ urlTab: 'model-status', storedTab: 'main' })).toBe('model-status');
  });

  it('does not let an invalid URL tab fall back to stored state by default', () => {
    expect(resolveUserPanelTab({ urlTab: 'bad-tab', storedTab: 'requests' })).toBe('main');
  });

  it('restores a valid stored tab only when explicitly allowed', () => {
    expect(resolveUserPanelTab({ urlTab: null, storedTab: 'requests', allowStoredTab: true })).toBe(
      'requests',
    );
  });

  it('falls back to main when URL and stored tab are invalid', () => {
    expect(
      resolveUserPanelTab({ urlTab: 'bad-tab', storedTab: 'other', allowStoredTab: true }),
    ).toBe('main');
  });

  it('updates tab in search while preserving unrelated query params', () => {
    expect(updateUserPanelTabSearch('?foo=bar&tab=main', 'model-status')).toBe(
      '?foo=bar&tab=model-status',
    );
  });
});
