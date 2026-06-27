import { expect, test, type Page } from 'playwright/test';

const providers = [
  {
    id: 101,
    createdAt: '2026-06-27T00:00:00Z',
    updatedAt: '2026-06-27T00:00:00Z',
    type: 'custom',
    name: 'Claude OK',
    config: { custom: { baseURL: 'https://ok.example.com', apiKey: 'sk-ok' } },
    supportedClientTypes: ['claude'],
  },
  {
    id: 102,
    createdAt: '2026-06-27T00:00:00Z',
    updatedAt: '2026-06-27T00:00:00Z',
    type: 'custom',
    name: 'Claude Duplicate',
    config: { custom: { baseURL: 'https://duplicate.example.com', apiKey: 'sk-dup' } },
    supportedClientTypes: ['claude'],
  },
  {
    id: 103,
    createdAt: '2026-06-27T00:00:00Z',
    updatedAt: '2026-06-27T00:00:00Z',
    type: 'custom',
    name: 'Claude Locked DB',
    config: { custom: { baseURL: 'https://locked.example.com', apiKey: 'sk-lock' } },
    supportedClientTypes: ['claude'],
  },
];

async function mockRouteApis(page: Page) {
  const routes: any[] = [];
  const createAttempts = new Map<number, number>();

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

    if (pathname === '/api/providers' || pathname === '/api/admin/providers') {
      return json(providers);
    }

    if (pathname === '/api/routes' || pathname === '/api/admin/routes') {
      if (route.request().method() === 'GET') {
        return json(routes);
      }

      if (route.request().method() === 'POST') {
        const payload = route.request().postDataJSON();
        const providerID = Number(payload.providerID ?? payload.providerId ?? payload.provider_id);
        if (!Number.isFinite(providerID)) {
          return json({ error: `missing provider id in payload: ${JSON.stringify(payload)}` }, 400);
        }
        const attempts = (createAttempts.get(providerID) ?? 0) + 1;
        createAttempts.set(providerID, attempts);

        if (providerID === 102 && attempts <= 2) {
          return json({ error: 'provider already has a claude route' }, 409);
        }

        if (providerID === 103 && attempts <= 2) {
          return json({ message: 'database is locked, retry after migration' }, 500);
        }

        const created = {
          id: routes.length + 1,
          createdAt: '2026-06-27T00:00:00Z',
          updatedAt: '2026-06-27T00:00:00Z',
          isEnabled: true,
          isNative: true,
          projectID: payload.projectID,
          clientType: payload.clientType,
          providerID,
          position: payload.position,
          weight: payload.weight,
          retryConfigID: payload.retryConfigID,
        };
        routes.push(created);
        return json(created);
      }
    }

    if (
      pathname === '/api/projects' ||
      pathname === '/api/admin/projects' ||
      pathname === '/api/admin/routing-strategies' ||
      pathname === '/api/requests/active' ||
      pathname === '/api/admin/requests/active'
    ) {
      return json([]);
    }

    if (pathname === '/api/provider-stats' || pathname === '/api/admin/provider-stats') {
      return json({});
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

test('claude bulk add keeps failed providers selected with actionable failure details', async ({
  page,
}, testInfo) => {
  await page.goto('/routes/claude');

  await expect(page.getByText(/Available Providers|可用提供方/)).toBeVisible({ timeout: 10000 });
  await page.getByText(/Select visible providers \(3\)|选择当前可见提供方（3）/).click();
  await page.getByRole('button', { name: /Add 3 selected|添加所选 3 个/ }).click();

  const alert = page.getByRole('alert');
  await expect(alert).toContainText(/Failed to add 2\/3 provider\(s\)|2\/3 个提供方添加失败/);
  await expect(alert).toContainText('Claude Duplicate');
  await expect(alert).toContainText('provider already has a claude route');
  await expect(alert).toContainText('Claude Locked DB');
  await expect(alert).toContainText('database is locked, retry after migration');
  await expect(page.getByRole('button', { name: /Retry failed items \(2\)|重试失败项（2）/ })).toBeVisible();
  await expect(page.getByLabel('Select available provider Claude Duplicate')).toBeChecked();
  await expect(page.getByLabel('Select available provider Claude Locked DB')).toBeChecked();
  await expect(page.getByLabel('Select available provider Claude OK')).toHaveCount(0);

  const screenshotPath = testInfo.outputPath('claude-bulk-add-failure-details.png');
  await page.screenshot({ path: screenshotPath, fullPage: true });
  await testInfo.attach('claude bulk add failure details', {
    path: screenshotPath,
    contentType: 'image/png',
  });

  await page.getByRole('button', { name: /Retry failed items \(2\)|重试失败项（2）/ }).click();
  await expect(alert).toHaveCount(0);
  await expect(page.getByText(/No routes configured|未为 Claude 配置路由/)).toHaveCount(0);
  await expect(page.getByLabel('Select route for Claude OK')).toBeVisible();
  await expect(page.getByLabel('Select route for Claude Duplicate')).toBeVisible();
  await expect(page.getByLabel('Select route for Claude Locked DB')).toBeVisible();
  await expect(page.getByLabel('Select available provider Claude Duplicate')).toHaveCount(0);
  await expect(page.getByLabel('Select available provider Claude Locked DB')).toHaveCount(0);

  const retrySuccessScreenshotPath = testInfo.outputPath('claude-bulk-add-retry-success.png');
  await page.screenshot({ path: retrySuccessScreenshotPath, fullPage: true });
  await testInfo.attach('claude bulk add retry success', {
    path: retrySuccessScreenshotPath,
    contentType: 'image/png',
  });
});
