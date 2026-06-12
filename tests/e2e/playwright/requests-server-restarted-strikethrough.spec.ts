/**
 * Playwright E2E Test: Requests Page - Server Restarted rows use strikethrough styling
 *
 * Regression test for #484.
 *
 * 使用方式：
 *   npx playwright test -c playwright.config.ts requests-server-restarted-strikethrough.spec.ts --project=e2e-chromium
 */
import { expect, test } from 'playwright/test';

import { BASE, loginToAdminUI } from './helpers';

const mockRequest = {
  id: 484,
  createdAt: '2026-04-08T06:00:00Z',
  updatedAt: '2026-04-08T06:00:05Z',
  instanceID: 'old-instance',
  requestID: 'req_issue_484',
  sessionID: 'session_issue_484',
  clientType: 'claude',
  requestModel: 'claude-sonnet-4-20250514',
  responseModel: '',
  startTime: '2026-04-08T06:00:00Z',
  endTime: '2026-04-08T06:00:05Z',
  duration: 5_000_000_000,
  ttft: 0,
  isStream: false,
  status: 'FAILED',
  statusCode: 500,
  requestInfo: null,
  responseInfo: null,
  error: 'Server restarted',
  proxyUpstreamAttemptCount: 1,
  finalProxyUpstreamAttemptID: 0,
  routeID: 1,
  providerID: 1,
  projectID: 0,
  inputTokenCount: 0,
  outputTokenCount: 0,
  cacheReadCount: 0,
  cacheWriteCount: 0,
  cache5mWriteCount: 0,
  cache1hWriteCount: 0,
  modelPriceId: 0,
  multiplier: 10000,
  cost: 0,
  apiTokenID: 0,
};

test('requests page marks Server restarted failures with strikethrough styling', async ({
  page,
}) => {
  await loginToAdminUI(page);

  await page.route('**/api/admin/requests**', async (route) => {
    const url = new URL(route.request().url());

    if (url.pathname.endsWith('/requests/count')) {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: '1',
      });
      return;
    }

    if (url.pathname.endsWith('/requests/active')) {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: '[]',
      });
      return;
    }

    if (url.pathname.endsWith('/requests')) {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          items: [mockRequest],
          hasMore: false,
          firstId: mockRequest.id,
          lastId: mockRequest.id,
        }),
      });
      return;
    }

    await route.fallback();
  });

  await page.goto(`${BASE}/requests`);

  const restartedRow = page.locator('[data-server-restarted-request="true"]').first();
  await expect(restartedRow).toBeVisible({ timeout: 15000 });
  await expect(restartedRow).toContainText('claude-sonnet-4-20250514');
  await expect(restartedRow).toHaveClass(/line-through/);
});
