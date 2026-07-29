import { expect, test, type Page, type Route } from '@playwright/test';

async function json(route: Route, body: unknown, status = 200) {
  await route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) });
}

async function installCommonMocks(page: Page) {
  await page.addInitScript(() => {
    localStorage.setItem('maxx-admin-token', 'mock-token');
    localStorage.setItem('maxx-ui-language', 'zh');
  });

  await page.route('**/api/settings', (route) =>
    json(route, {
      api_token_auth_enabled: 'true',
      force_project_binding: 'false',
      ui_multitenant_enabled: 'true',
    }),
  );
  await page.route('**/api/admin/settings', (route) =>
    json(route, {
      api_token_auth_enabled: 'true',
      force_project_binding: 'false',
      ui_multitenant_enabled: 'true',
    }),
  );
  await page.route('**/api/admin/auth/status', (route) =>
    json(route, {
      authEnabled: true,
      user: { id: 1, username: 'admin', tenantID: 1, role: 'admin' },
    }),
  );
  await page.route('**/api/proxy-status', (route) =>
    json(route, { running: true, address: '127.0.0.1', port: 9880, version: 'e2e' }),
  );
  await page.route('**/api/admin/proxy-status', (route) =>
    json(route, { running: true, address: '127.0.0.1', port: 9880, version: 'e2e' }),
  );
  await page.route('**/api/admin/provider-stats**', (route) => json(route, []));
  await page.route('**/api/admin/streaming-requests/counts**', (route) => json(route, {}));
}

test.use({ viewport: { width: 1440, height: 1000 }, locale: 'zh-CN' });

test.describe('Invite code and routing strategy UI regressions', () => {
  test('invite code usages dialog removes the prefix placeholder while keeping the code visible', async ({
    page,
  }) => {
    await installCommonMocks(page);
    await page.route('**/api/admin/invite-codes', (route) =>
      json(route, [
        {
          id: 1,
          createdAt: '2026-07-29T00:00:00Z',
          updatedAt: '2026-07-29T00:00:00Z',
          tenantID: 1,
          codePrefix: 'BKAX2GFJ',
          status: 'active',
          maxUses: 1,
          usedCount: 1,
          createdByUserID: 1,
          note: '',
        },
      ]),
    );
    await page.route('**/api/admin/invite-codes/1/usages', (route) =>
      json(route, [
        {
          id: 11,
          createdAt: '2026-07-29T00:00:00Z',
          updatedAt: '2026-07-29T00:00:00Z',
          tenantID: 1,
          inviteCodeID: 1,
          userID: 2,
          username: 'tester',
          usedAt: '2026-07-29T00:00:00Z',
          ip: '127.0.0.1',
          userAgent: 'playwright',
          result: 'success',
        },
      ]),
    );

    await page.goto('/invite-codes');
    await page.getByRole('button', { name: '查看使用记录' }).click();

    const dialog = page.getByRole('dialog');
    await expect(dialog.getByRole('heading', { name: '邀请码 BKAX2GFJ 的使用记录' })).toBeVisible();
    await expect(dialog.getByText('前缀: BKAX2GFJ')).toHaveCount(0);
    await expect(dialog.getByText('tester')).toBeVisible();
  });

  test('routing strategy content aligns to the shared page padding instead of an extra centered wrapper', async ({
    page,
  }) => {
    await installCommonMocks(page);
    await page.route('**/api/admin/projects', (route) => json(route, []));
    await page.route('**/api/admin/routing-strategies', (route) =>
      json(route, [
        {
          id: 1,
          createdAt: '2026-07-29T00:00:00Z',
          updatedAt: '2026-07-29T00:00:00Z',
          tenantID: 1,
          projectID: 0,
          type: 'priority',
          config: null,
        },
      ]),
    );

    await page.goto('/routing-strategies');

    const scroller = page.locator('.flex-1.overflow-auto.p-4.md\\:p-6');
    const card = page.getByText('全部策略').locator('xpath=ancestor::*[contains(@class, "rounded")][1]');
    await expect(card).toBeVisible();

    const scrollerBox = await scroller.boundingBox();
    const cardBox = await card.boundingBox();
    expect(scrollerBox).not.toBeNull();
    expect(cardBox).not.toBeNull();
    expect(Math.round(cardBox!.x - scrollerBox!.x)).toBe(24);
  });
});
