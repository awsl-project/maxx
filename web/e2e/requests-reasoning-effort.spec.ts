import { expect, test, type Page, type Route } from '@playwright/test';

async function json(route: Route, body: unknown) {
  await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(body) });
}

async function installRequestPageMocks(page: Page) {
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
  await page.route('**/api/settings', (route) =>
    json(route, {
      api_token_auth_enabled: 'false',
      force_project_binding: 'false',
      ui_multitenant_enabled: 'false',
    }),
  );
  await page.route('**/api/providers', (route) => json(route, []));
  await page.route('**/api/projects', (route) => json(route, []));
  await page.route('**/api/api-tokens', (route) => json(route, []));
  await page.route(/\/api\/admin\/requests\/cleanup-failed-count(?:\?.*)?$/, (route) =>
    json(route, 0),
  );
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

  const modelHeader = page.getByRole('columnheader', { name: '模型', exact: true });
  const reasoningHeader = page.getByRole('columnheader', { name: '思考深度', exact: true });
  const row = page.locator('[data-request-row="true"]');

  await expect(modelHeader).toBeVisible();
  await expect(reasoningHeader).toBeVisible();
  await expect(row.getByText('gpt-5.6-sol', { exact: true })).toBeVisible();
  await expect(row.getByText('high', { exact: true })).toBeVisible();

  const modelBox = await modelHeader.boundingBox();
  const reasoningBox = await reasoningHeader.boundingBox();
  expect(modelBox).not.toBeNull();
  expect(reasoningBox).not.toBeNull();
  expect(modelBox!.width).toBeLessThan(220);
  expect(Math.abs(reasoningBox!.x - (modelBox!.x + modelBox!.width))).toBeLessThan(2);

  await page.screenshot({ path: testInfo.outputPath('requests-reasoning-effort.png') });
});
