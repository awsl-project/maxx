/**
 * Playwright E2E Test: Requests Page - auth/session isolation
 *
 * Regression test for issue #438.
 *
 * 使用方式：
 *   npx playwright test -c playwright.config.ts requests-auth-switch-isolation.spec.ts --project=e2e-chromium
 */
import { expect, test } from 'playwright/test';

const UI_BASE = 'http://127.0.0.1:4173';

const ADMIN_TOKEN = 'admin-token';
const TENANT_TOKEN = 'tenant-2-token';

const adminUser = {
  id: 1,
  username: 'admin',
  tenantID: 1,
  tenantName: 'Admin Tenant',
  role: 'admin',
};

const tenantUser = {
  id: 2,
  username: 'tenant-two-admin',
  tenantID: 2,
  tenantName: 'Tenant Two',
  role: 'admin',
};

const adminTokenOptions = [{ id: 101, name: 'Admin Token' }];
const tenantTokenOptions = [{ id: 202, name: 'Tenant Two Token' }];
const providers = [{ id: 1, name: 'Claude Provider', type: 'custom' }];

function buildRequest(id: number, model: string, apiTokenID: number) {
  return {
    id,
    createdAt: '2026-04-10T05:05:00Z',
    updatedAt: '2026-04-10T05:05:05Z',
    instanceID: 'instance-test',
    requestID: `req-${id}`,
    sessionID: `session-${id}`,
    clientType: 'claude',
    requestModel: model,
    responseModel: '',
    startTime: '2026-04-10T05:05:00Z',
    endTime: '2026-04-10T05:05:05Z',
    duration: 5_000_000_000,
    ttft: 0,
    isStream: false,
    status: 'COMPLETED',
    statusCode: 200,
    requestInfo: null,
    responseInfo: null,
    error: '',
    proxyUpstreamAttemptCount: 1,
    finalProxyUpstreamAttemptID: 0,
    routeID: 1,
    providerID: 1,
    projectID: 0,
    inputTokenCount: 32,
    outputTokenCount: 64,
    cacheReadCount: 0,
    cacheWriteCount: 0,
    cache5mWriteCount: 0,
    cache1hWriteCount: 0,
    modelPriceId: 0,
    multiplier: 10000,
    cost: 0,
    apiTokenID,
  };
}

function readBearerToken(header: string | null): string | null {
  if (!header) {
    return null;
  }
  const match = header.match(/^Bearer\s+(.+)$/i);
  return match?.[1] ?? null;
}

test('requests page does not leak cached requests or token filters across tenant logins', async ({
  page,
}) => {
  const adminRequest = buildRequest(4381, 'admin-tenant-model', 101);
  const tenantRequest = buildRequest(4382, 'tenant-two-model', 202);

  let tenantTokensResolved = false;

  await page.route('**/api/**', async (route) => {
    const url = new URL(route.request().url());
    const bearer = readBearerToken(await route.request().headerValue('authorization'));

    const session =
      bearer === ADMIN_TOKEN
        ? { user: adminUser, requests: [adminRequest], tokens: adminTokenOptions }
        : bearer === TENANT_TOKEN
          ? { user: tenantUser, requests: [tenantRequest], tokens: tenantTokenOptions }
          : null;

    if (url.pathname === '/api/admin/auth/status') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ authEnabled: true, user: session?.user ?? null }),
      });
      return;
    }

    if (url.pathname === '/api/admin/auth/login' && route.request().method() === 'POST') {
      const payload = route.request().postDataJSON() as { username?: string };
      if (payload.username === 'admin') {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ success: true, token: ADMIN_TOKEN, user: adminUser }),
        });
        return;
      }
      if (payload.username === 'tenant-two-admin') {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ success: true, token: TENANT_TOKEN, user: tenantUser }),
        });
        return;
      }
      await route.fulfill({
        status: 401,
        contentType: 'application/json',
        body: JSON.stringify({ error: 'invalid credentials' }),
      });
      return;
    }

    if (url.pathname === '/api/settings') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          api_token_auth_enabled: 'true',
          force_project_binding: 'false',
        }),
      });
      return;
    }

    if (!session) {
      await route.fulfill({
        status: 401,
        contentType: 'application/json',
        body: JSON.stringify({ error: 'unauthorized' }),
      });
      return;
    }

    if (url.pathname === '/api/admin/providers') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(providers),
      });
      return;
    }

    if (url.pathname === '/api/projects') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify([]),
      });
      return;
    }

    if (url.pathname === '/api/api-tokens') {
      if (bearer === TENANT_TOKEN) {
        await new Promise((resolve) => setTimeout(resolve, 5000));
        tenantTokensResolved = true;
      }
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(session.tokens),
      });
      return;
    }

    if (url.pathname === '/api/admin/requests/count') {
      if (bearer === TENANT_TOKEN) {
        await new Promise((resolve) => setTimeout(resolve, 500));
      }
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(session.requests.length),
      });
      return;
    }

    if (url.pathname === '/api/admin/requests') {
      if (bearer === TENANT_TOKEN) {
        await new Promise((resolve) => setTimeout(resolve, 500));
      }

      const requestedTokenId = url.searchParams.get('apiTokenId');
      const items = requestedTokenId
        ? session.requests.filter((item) => String(item.apiTokenID) === requestedTokenId)
        : session.requests;

      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          items,
          hasMore: false,
          firstId: items[0]?.id ?? 0,
          lastId: items[items.length - 1]?.id ?? 0,
        }),
      });
      return;
    }

    await route.fulfill({
      status: 404,
      contentType: 'application/json',
      body: JSON.stringify({ error: `Unhandled API route: ${url.pathname}` }),
    });
  });

  await page.goto(`${UI_BASE}/requests`);

  await expect(page.locator('input[type="text"]')).toBeVisible({ timeout: 10000 });
  await page.fill('input[type="text"]', 'admin');
  await page.fill('input[type="password"]', 'test123');
  await page.locator('button[type="submit"]').click();

  await expect(page.locator('body')).toContainText('admin-tenant-model', { timeout: 10000 });

  await page.evaluate(() => {
    localStorage.setItem('maxx-requests-filter-mode', 'token');
    localStorage.setItem('maxx-requests-token-filter', '101');
    localStorage.setItem('maxx-requests-filter-mode:tenant-1:user-1', 'token');
    localStorage.setItem('maxx-requests-token-filter:tenant-1:user-1', '101');
  });

  await page.reload();
  await expect(page.locator('body')).toContainText('admin-tenant-model', { timeout: 10000 });

  const menuButton = page.locator('button[aria-haspopup="menu"]').last();
  await menuButton.click();
  const logoutItem = page.locator('[role="menuitem"]').filter({ hasText: /Log out|退出登录/ }).first();
  await expect(logoutItem).toBeVisible({ timeout: 5000 });
  await logoutItem.click();
  await expect(page.locator('input[type="text"]')).toBeVisible({ timeout: 5000 });

  await page.fill('input[type="text"]', 'tenant-two-admin');
  await page.fill('input[type="password"]', 'test123');
  await page.locator('button[type="submit"]').click();

  await expect(page.locator('body')).not.toContainText('admin-tenant-model', { timeout: 1200 });
  await expect(page.locator('body')).toContainText('tenant-two-model', { timeout: 3000 });
  expect(tenantTokensResolved).toBeFalsy();
});
