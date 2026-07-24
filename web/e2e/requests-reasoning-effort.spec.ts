import { expect, test, type Page, type Route } from '@playwright/test';

async function json(route: Route, body: unknown) {
  await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(body) });
}

async function installRequestPageMocks(
  page: Page,
  options: {
    cleanupFailedCount?: number | (() => number);
    adminSettings?: Record<string, string>;
  } = {},
) {
  await page.addInitScript(() => {
    localStorage.setItem('maxx-admin-token', 'mock-token');
    localStorage.setItem('maxx-ui-language', 'zh');
  });

  const now = new Date().toISOString();
  const request = {
    id: 1,
    createdAt: now,
    updatedAt: now,
    instanceID: 'e2e',
    requestID: 'request-reasoning-e2e',
    sessionID: 'session-reasoning-e2e',
    clientType: 'codex',
    requestModel: 'gpt-5.6-sol',
    responseModel: 'gpt-5.6-sol',
    reasoningEffort: 'high',
    startTime: now,
    endTime: now,
    duration: 3_310_000_000,
    ttft: 1_020_000_000,
    isStream: true,
    status: 'COMPLETED',
    statusCode: 200,
    requestInfo: null,
    responseInfo: null,
    error: '',
    proxyUpstreamAttemptCount: 1,
    finalProxyUpstreamAttemptID: 1,
    routeID: 1,
    providerID: 0,
    projectID: 0,
    inputTokenCount: 493,
    outputTokenCount: 43,
    cacheReadCount: 72_200,
    cacheWriteCount: 0,
    cache5mWriteCount: 0,
    cache1hWriteCount: 0,
    modelPriceId: 0,
    multiplier: 10_000,
    cost: 0,
    apiTokenID: 0,
  };

  await page.route('**/api/admin/auth/status', (route) =>
    json(route, {
      authEnabled: true,
      user: { id: 1, username: 'admin', tenantID: 1, role: 'admin' },
    }),
  );
  const adminSettings = options.adminSettings ?? {};

  await page.route('**/api/settings', (route) =>
    json(route, {
      api_token_auth_enabled: 'false',
      force_project_binding: 'false',
      ui_multitenant_enabled: 'false',
    }),
  );
  await page.route('**/api/admin/settings', (route) => json(route, adminSettings));
  await page.route(/\/api\/admin\/settings\/[^/]+$/, async (route) => {
    const key = decodeURIComponent(new URL(route.request().url()).pathname.split('/').pop() ?? '');
    if (route.request().method() === 'PUT' || route.request().method() === 'POST') {
      const body = route.request().postDataJSON() as { value?: string };
      adminSettings[key] = body.value ?? '';
      return json(route, { key, value: adminSettings[key] });
    }
    return json(route, { key, value: adminSettings[key] ?? '' });
  });
  await page.route('**/api/providers', (route) => json(route, []));
  await page.route('**/api/projects', (route) => json(route, []));
  await page.route('**/api/api-tokens', (route) => json(route, []));
  await page.route(/\/api\/admin\/requests\/cleanup-failed-count(?:\?.*)?$/, (route) => {
    const count = options.cleanupFailedCount;
    return json(route, typeof count === 'function' ? count() : (count ?? 0));
  });
  await page.route(/\/api\/admin\/requests\/count(?:\?.*)?$/, (route) => json(route, 1));
  await page.route(/\/api\/admin\/requests(?:\?.*)?$/, (route) =>
    json(route, { items: [request], hasMore: false, firstId: 1, lastId: 1 }),
  );
}

test.use({ viewport: { width: 1548, height: 1018 }, locale: 'zh-CN' });

test('request table shows reasoning effort beside a compact model column', async ({
  page,
}, testInfo) => {
  await installRequestPageMocks(page);
  await page.goto('/requests');

  const modelHeader = page.getByRole('columnheader', { name: /模型/ });
  const protocolHeader = page.getByRole('columnheader', { name: /协议/ });
  const reasoningHeader = page.getByRole('columnheader', { name: /思考深度/ });
  const row = page.locator('[data-request-row="true"]');

  await expect(modelHeader).toBeVisible();
  await expect(protocolHeader).toBeVisible();
  await expect(reasoningHeader).toBeVisible();
  await expect(row.getByText('gpt-5.6-sol', { exact: true })).toBeVisible();
  await expect(row.getByText('SSE', { exact: true })).toBeVisible();
  await expect(row.getByText('high', { exact: true })).toBeVisible();

  const modelBox = await modelHeader.boundingBox();
  const protocolBox = await protocolHeader.boundingBox();
  const reasoningBox = await reasoningHeader.boundingBox();
  expect(modelBox).not.toBeNull();
  expect(protocolBox).not.toBeNull();
  expect(reasoningBox).not.toBeNull();
  expect(modelBox!.width).toBeLessThan(220);
  expect(protocolBox!.x).toBeGreaterThan(modelBox!.x);
  expect(reasoningBox!.x).toBeGreaterThan(protocolBox!.x);

  await page.screenshot({ path: testInfo.outputPath('requests-reasoning-effort.png') });
});

test('refresh enables cleanup failed button when failed records already exist without new requests', async ({
  page,
}) => {
  let cleanupFailedCount = 0;
  await installRequestPageMocks(page, { cleanupFailedCount: () => cleanupFailedCount });
  await page.goto('/requests');

  const cleanupButton = page.getByRole('button', { name: '清理失败记录' });
  await expect(cleanupButton).toBeDisabled();

  cleanupFailedCount = 2;
  await page.getByRole('button', { name: '刷新' }).click();

  await expect(cleanupButton).toBeEnabled();
});

test('request column visibility survives page restart from backend settings', async ({ page }) => {
  const adminSettings: Record<string, string> = {};
  await installRequestPageMocks(page, { adminSettings });
  await page.goto('/requests');

  await expect(page.getByRole('columnheader', { name: /费用/ })).toBeVisible();
  await page.getByRole('button', { name: '列显示与排序' }).click();
  await page.getByRole('checkbox', { name: '费用' }).uncheck();
  await expect(page.getByRole('columnheader', { name: /费用/ })).toBeHidden();
  await expect
    .poll(() => adminSettings['ui.requests.table.columns.tenant-1.user-1'])
    .toContain('"cost":false');

  await page.evaluate(() => localStorage.clear());
  await page.reload();

  await expect(page.getByRole('columnheader', { name: /费用/ })).toBeHidden();
});
