export const USER_PANEL_TABS = ['main', 'requests'] as const;

export type UserPanelTab = (typeof USER_PANEL_TABS)[number];

const DEFAULT_USER_PANEL_TAB: UserPanelTab = 'main';
const STORAGE_PREFIX = 'maxx:user-panel:active-tab';

export function isUserPanelTab(value: string | null | undefined): value is UserPanelTab {
  return value === 'main' || value === 'requests';
}

export function getUserPanelTabStorageKey(userId: number | string | null | undefined): string {
  const scope = userId === null || userId === undefined || userId === '' ? 'anonymous' : String(userId);
  return `${STORAGE_PREFIX}:${scope}`;
}

export function resolveUserPanelTab(params: {
  urlTab?: string | null;
  storedTab?: string | null;
}): UserPanelTab {
  if (isUserPanelTab(params.urlTab)) {
    return params.urlTab;
  }
  if (isUserPanelTab(params.storedTab)) {
    return params.storedTab;
  }
  return DEFAULT_USER_PANEL_TAB;
}

export function updateUserPanelTabSearch(search: string, tab: UserPanelTab): string {
  const params = new URLSearchParams(search);
  params.set('tab', tab);
  const next = params.toString();
  return next ? `?${next}` : '';
}
