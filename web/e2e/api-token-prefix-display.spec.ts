import { expect, test, type Route } from '@playwright/test';

async function json(route: Route, body: unknown) {
  await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(body) });
}

test.use({ viewport: { width: 1440, height: 900 }, locale: 'zh-CN' });

test('api token prefixes render in a stable compact display format', async ({ page }) => {
  await page.addInitScript(() => {
    localStorage.setItem('maxx-admin-token', 'mock-token');
    localStorage.setItem('maxx-ui-language', 'zh');
  });

  await page.route('**/api/admin/auth/status', (route) =>
    json(route, {
      authEnabled: true,
      user: { id: 1, username: 'admin', tenantID: 1, role: 'admin' },
    }),
  );
  await page.route('**/api/admin/settings', (route) =>
    json(route, { api_token_auth_enabled: 'true' }),
  );
  await page.route('**/api/admin/proxy-status', (route) =>
    json(route, { running: true, address: '127.0.0.1', port: 9880, version: 'e2e' }),
  );
  await page.route('**/api/projects', (route) => json(route, []));
  await page.route('**/api/admin/api-tokens', (route) =>
    json(route, [
      {
        id: 1,
        name: 'Generated token',
        description: '',
        token: 'maxx76dd169738b9f4269a418f4b4d763a5c9dd1a6a9ee6a13d9a07f0a9f1234',
        tokenPrefix: 'maxx76dd169738b9f4269a4...',
        projectID: 0,
        project: null,
        isEnabled: true,
        usageCount: 0,
        recentIP: '',
        lastUsedAt: null,
        expiresAt: null,
        createdAt: new Date().toISOString(),
        updatedAt: new Date().toISOString(),
      },
      {
        id: 2,
        name: 'Imported token',
        description: '',
        token: 'maxxexp-full-token-kept-for-copy',
        tokenPrefix: 'maxxexp',
        projectID: 0,
        project: null,
        isEnabled: true,
        usageCount: 0,
        recentIP: '',
        lastUsedAt: null,
        expiresAt: null,
        createdAt: new Date().toISOString(),
        updatedAt: new Date().toISOString(),
      },
    ]),
  );

  await page.goto('/api-tokens');

  const generatedPrefix = page.getByText('maxx76dd…69a4', { exact: true });
  const importedPrefix = page.getByText('maxxexp', { exact: true });

  await expect(generatedPrefix).toBeVisible();
  await expect(importedPrefix).toBeVisible();
  await expect(page.getByText('maxx76dd169738b9f4269a4...', { exact: true })).toBeHidden();
  await expect(generatedPrefix).toHaveAttribute('title', 'maxx76dd169738b9f4269a4...');
});
