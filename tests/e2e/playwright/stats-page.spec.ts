import { expect, test, type Page } from 'playwright/test';

type UsageStat = {
  id: number;
  createdAt: string;
  timeBucket: string;
  granularity: string;
  routeID: number;
  providerID: number;
  projectID: number;
  apiTokenID: number;
  clientType: string;
  model: string;
  totalRequests: number;
  successfulRequests: number;
  failedRequests: number;
  inputTokens: number;
  outputTokens: number;
  cacheRead: number;
  cacheWrite: number;
  cost: number;
  totalDurationMs: number;
  totalTtftMs: number;
};

function buildUsageStats(): UsageStat[] {
  const now = new Date();
  return Array.from({ length: 24 }, (_, index) => {
    const bucket = new Date(now.getTime() - (23 - index) * 60 * 60 * 1000);
    return {
      id: index + 1,
      createdAt: now.toISOString(),
      timeBucket: bucket.toISOString(),
      granularity: 'hour',
      routeID: 100 + index,
      providerID: (index % 3) + 1,
      projectID: index % 2 === 0 ? 11 : 12,
      apiTokenID: index % 2 === 0 ? 21 : 22,
      clientType: index % 2 === 0 ? 'openai' : 'claude',
      model: index % 2 === 0 ? 'gpt-5' : 'claude-sonnet-4',
      totalRequests: 120 + index,
      successfulRequests: 116 + index,
      failedRequests: 4,
      inputTokens: 12000 + index * 50,
      outputTokens: 6000 + index * 40,
      cacheRead: 3000 + index * 20,
      cacheWrite: 1200 + index * 10,
      cost: 250000000 + index * 1000000,
      totalDurationMs: 120000 + index * 100,
      totalTtftMs: 58000 + index * 50,
    };
  });
}

async function mockStatsPageApis(page: Page) {
  const usageStats = buildUsageStats();

  await page.route('**/api/admin/**', async (route) => {
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

    if (pathname === '/api/admin/settings') {
      return json({});
    }

    if (pathname === '/api/admin/proxy-status') {
      return json({ address: '127.0.0.1:9880', version: 'v0.1.1' });
    }

    if (pathname === '/api/admin/providers') {
      return json([
        { id: 1, name: 'Claude Pool', type: 'claude' },
        { id: 2, name: 'Codex Pool', type: 'codex' },
        { id: 3, name: 'Custom Pool', type: 'custom' },
      ]);
    }

    if (pathname === '/api/admin/projects') {
      return json([
        { id: 11, name: 'Project Alpha', slug: 'project-alpha' },
        { id: 12, name: 'Project Beta', slug: 'project-beta' },
      ]);
    }

    if (pathname === '/api/admin/api-tokens') {
      return json([
        { id: 21, name: 'Main Token' },
        { id: 22, name: 'Fallback Token' },
      ]);
    }

    if (pathname === '/api/admin/response-models') {
      return json(['gpt-5', 'claude-sonnet-4', 'gemini-2.5-pro']);
    }

    if (pathname === '/api/admin/usage-stats') {
      return json(usageStats);
    }

    return route.fulfill({
      status: 404,
      contentType: 'application/json',
      body: JSON.stringify({
        error: 'Unmocked admin endpoint',
        pathname,
        url: route.request().url(),
      }),
    });
  });
}

test.beforeEach(async ({ page }) => {
  await mockStatsPageApis(page);
});

test('desktop stats page renders summary and chart content', async ({ page }) => {
  await page.goto('/stats');

  await expect(page.getByTestId('stats-scroll-region')).toBeVisible();
  await expect(page.getByTestId('stats-chart-card')).toBeVisible();

  const cards = page.getByTestId('stats-summary-grid').locator(':scope > *');
  await expect(cards).toHaveCount(4);
});

test('stats page scroll region supports vertical scrolling', async ({ page }, testInfo) => {
  test.skip(
    testInfo.project.name !== 'mobile-chromium',
    'scroll overflow is exercised in the mobile layout targeted by this regression test',
  );

  await page.goto('/stats');

  const scrollRegion = page.getByTestId('stats-scroll-region');
  const chartCard = page.getByTestId('stats-chart-card');

  await expect(scrollRegion).toBeVisible();

  const before = await scrollRegion.evaluate((element) => ({
    clientHeight: element.clientHeight,
    scrollHeight: element.scrollHeight,
    scrollTop: element.scrollTop,
  }));

  expect(before.scrollTop).toBe(0);
  expect(before.scrollHeight).toBeGreaterThan(before.clientHeight);

  await scrollRegion.hover();
  await page.mouse.wheel(0, 1800);

  await expect
    .poll(() => scrollRegion.evaluate((element) => element.scrollTop), { timeout: 2000 })
    .toBeGreaterThan(0);

  const afterScrollTop = await scrollRegion.evaluate((element) => element.scrollTop);
  expect(afterScrollTop).toBeGreaterThan(0);

  await expect(chartCard).toBeInViewport();

  await testInfo.attach('stats-scroll-report-screenshot', {
    body: await page.screenshot({ fullPage: true }),
    contentType: 'image/png',
  });
});
