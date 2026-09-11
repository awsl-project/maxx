import { describe, expect, it } from 'vitest';
import type { MenuItem } from '@/types/sidebar';
import { shouldRenderSidebarItem } from './sidebar-renderer';

const standardItem = (key: string): MenuItem => ({
  type: 'standard',
  key,
  to: `/${key}`,
  icon: () => null,
  labelKey: `nav.${key}`,
});

describe('shouldRenderSidebarItem', () => {
  it('hides the proxy management tab by default', () => {
    expect(
      shouldRenderSidebarItem(standardItem('proxies'), {
        settings: {},
        isAdmin: true,
        authEnabled: true,
      }),
    ).toBe(false);
  });

  it('shows the proxy management tab only when the setting is true', () => {
    expect(
      shouldRenderSidebarItem(standardItem('proxies'), {
        settings: { ui_proxy_management_enabled: 'false' },
        isAdmin: true,
        authEnabled: true,
      }),
    ).toBe(false);

    expect(
      shouldRenderSidebarItem(standardItem('proxies'), {
        settings: { ui_proxy_management_enabled: 'true' },
        isAdmin: true,
        authEnabled: true,
      }),
    ).toBe(true);
  });

  it('keeps unrelated config tabs visible when proxy management is off', () => {
    expect(
      shouldRenderSidebarItem(standardItem('retry-configs'), {
        settings: {},
        isAdmin: true,
        authEnabled: true,
      }),
    ).toBe(true);
  });
});
