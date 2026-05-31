import { expect, test, type Page } from 'playwright/test';

async function mockRouteApis(page: Page) {
  await page.route('**/api/**', async (route) => {
    const url = new URL(route.request().url());
    const { pathname } = url;

    const json = (body: unknown, status = 200) =>
      route.fulfill({
        status,
        contentType: 'application/json',
        body: JSON.stringify(body),
      });

    if (pathname === '/api/admin/auth/status') {
      return json({ authEnabled: false });
    }

    if (pathname === '/api/settings' || pathname === '/api/admin/settings') {
      return json({ ui_multitenant_enabled: 'false' });
    }

    if (
      pathname === '/api/admin/routes' ||
      pathname === '/api/routes' ||
      pathname === '/api/admin/projects' ||
      pathname === '/api/projects' ||
      pathname === '/api/admin/providers' ||
      pathname === '/api/providers' ||
      pathname === '/api/admin/requests/active'
    ) {
      return json([]);
    }

    return route.fulfill({
      status: 404,
      contentType: 'application/json',
      body: JSON.stringify({ error: 'Unmocked endpoint', pathname }),
    });
  });
}

test.beforeEach(async ({ page }) => {
  await mockRouteApis(page);
});

test('route client tabs stay visible when multi-tenant UI is disabled', async ({ page }) => {
  await page.goto('/');

  for (const clientType of ['claude', 'openai', 'codex', 'gemini']) {
    await expect(page.locator(`a[href="/routes/${clientType}"]`)).toBeVisible({ timeout: 10000 });
  }
});

test('route pages remain accessible when multi-tenant UI is disabled', async ({ page }) => {
  await page.goto('/routes/claude');

  await expect(page).toHaveURL(/\/routes\/claude$/);
  await expect(page.locator('body')).toContainText(/Claude Routes|Claude 路由/i);
});
