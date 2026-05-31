import { expect, test, type Page } from 'playwright/test';

async function mockSettingsPageApis(page: Page) {
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

    if (pathname === '/api/admin/settings' || pathname === '/api/settings') {
      return json({});
    }

    if (
      pathname === '/api/admin/proxy-status' ||
      pathname === '/api/proxy-status'
    ) {
      return json({
        running: true,
        address: '127.0.0.1',
        port: 9880,
        version: 'v0.13.60',
      });
    }

    if (pathname === '/api/admin/providers' || pathname === '/api/providers') {
      return json([]);
    }

    if (pathname === '/api/admin/routes' || pathname === '/api/routes') {
      return json([]);
    }

    if (
      pathname === '/api/admin/api-tokens' ||
      pathname === '/api/api-tokens'
    ) {
      return json([]);
    }

    if (pathname === '/api/admin/projects' || pathname === '/api/projects') {
      return json([]);
    }

    if (
      pathname === '/api/admin/model-mappings' ||
      pathname === '/api/model-mappings'
    ) {
      return json([]);
    }

    if (
      pathname === '/api/admin/response-models' ||
      pathname === '/api/response-models'
    ) {
      return json([]);
    }

    return json({});
  });
}

test.beforeEach(async ({ page }) => {
  await mockSettingsPageApis(page);
});

test('settings theme tabs open luxury group when persisted theme is luxury', async ({
  page,
}) => {
  await page.goto('/settings', {
    waitUntil: 'domcontentloaded',
  });
  await page.evaluate(() => localStorage.setItem('maxx-ui-theme', 'hermes'));
  await page.reload({ waitUntil: 'domcontentloaded' });

  const luxuryTab = page.getByRole('tab', { name: /Luxury|奢华/i });
  await expect(luxuryTab).toBeVisible();
  await expect(luxuryTab).toHaveAttribute('aria-selected', 'true');
  await expect(
    page.getByRole('button', { name: /Select Hermès theme/i }),
  ).toBeVisible();
});

test('settings theme tabs switch to luxury group when theme changes from default to luxury', async ({
  page,
}) => {
  await page.goto('/settings', { waitUntil: 'domcontentloaded' });

  const luxuryTab = page.getByRole('tab', { name: /Luxury|奢华/i });
  await expect(luxuryTab).toBeVisible();
  await expect(luxuryTab).toHaveAttribute('aria-selected', 'false');

  await luxuryTab.click();
  await page.getByRole('button', { name: /Select Hermès theme/i }).click();

  await expect(page.getByRole('tab', { name: /Luxury|奢华/i })).toHaveAttribute(
    'aria-selected',
    'true',
  );
});
