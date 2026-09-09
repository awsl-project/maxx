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

    if (pathname === '/api/admin/proxy-status' || pathname === '/api/proxy-status') {
      return json({ address: '127.0.0.1:9880', version: 'v0.1.1' });
    }

    if (pathname === '/api/admin/providers' || pathname === '/api/providers') {
      return json([
        { id: 1, name: 'Claude Pool', type: 'claude' },
        { id: 2, name: 'Codex Pool', type: 'codex' },
        { id: 3, name: 'Custom Pool', type: 'custom' },
      ]);
    }

    if (pathname === '/api/admin/projects' || pathname === '/api/projects') {
      return json([
        { id: 11, name: 'Project Alpha', slug: 'project-alpha' },
        { id: 12, name: 'Project Beta', slug: 'project-beta' },
      ]);
    }

    if (pathname === '/api/admin/api-tokens' || pathname === '/api/api-tokens') {
      return json([
        { id: 21, name: 'Main Token' },
        { id: 22, name: 'Fallback Token' },
      ]);
    }

    if (pathname === '/api/admin/response-models' || pathname === '/api/response-models') {
      return json(Array.from({ length: 40 }, (_, index) => `example-model-${index + 1}`));
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

for (const viewport of [
  { width: 320, height: 568 },
  { width: 390, height: 844 },
  { width: 739, height: 500 },
  { width: 739, height: 300 },
  { width: 768, height: 400 },
  { width: 1024, height: 500 },
]) {
  test(`stats chart bottom is reachable at ${viewport.width}x${viewport.height}`, async ({
    page,
  }, testInfo) => {
    await page.setViewportSize(viewport);
    await page.goto('/stats');

    const chartCard = page.getByTestId('stats-chart-card');
    const scrollRegion = page.getByTestId(
      viewport.width < 768 ? 'stats-scroll-region' : 'stats-results-region',
    );

    await expect(chartCard.getByRole('application')).toBeVisible();
    await expect
      .poll(() => chartCard.evaluate((element) => element.clientHeight))
      .toBeGreaterThan(400);
    await expect
      .poll(() => scrollRegion.evaluate((element) => element.scrollHeight - element.clientHeight))
      .toBeGreaterThan(0);

    await scrollRegion.hover();
    await page.mouse.wheel(0, 10000);

    await expect
      .poll(() => scrollRegion.evaluate((element) => element.scrollTop))
      .toBeGreaterThan(0);
    await expect
      .poll(() =>
        chartCard.evaluate((element) => {
          const rect = element.getBoundingClientRect();
          return rect.bottom > 0 && rect.bottom <= window.innerHeight;
        }),
      )
      .toBe(true);

    const dimensions = await chartCard.evaluate((element) => ({
      height: element.clientHeight,
      contentHeight: element.scrollHeight,
    }));
    expect(dimensions.contentHeight).toBeLessThanOrEqual(dimensions.height + 1);
    await page.screenshot({ path: testInfo.outputPath('stats-bottom.png') });
  });
}

test('model chips collapse by available width and only promote the selection when collapsed', async ({
  page,
}) => {
  await page.route('**/api/response-models', (route) =>
    route.fulfill({
      json: ['model-a', 'model-b', 'model-c', 'model-d', 'model-e', 'model-f'],
    }),
  );
  await page.setViewportSize({ width: 739, height: 500 });
  await page.goto('/stats');
  const row = page.getByTestId('stats-model-filter-row');
  const picker = row.getByRole('button', { name: 'Select Model', exact: true });
  await expect(row.getByRole('button')).toHaveCount(6);
  await expect(picker).toHaveCount(0);
  await row.getByRole('button', { name: 'model-f', exact: true }).click();
  await expect(row.getByRole('button').first()).toHaveText('model-a');

  await page.setViewportSize({ width: 320, height: 568 });
  await expect(picker).toBeVisible();
  await expect(row.getByRole('button').first()).toHaveText('model-f');
  const visibleCount = (await row.getByRole('button').count()) - 1;
  expect(visibleCount).toBeGreaterThan(0);
  await expect(picker).toHaveText(`+${6 - visibleCount}`);
  const bounds = await row.evaluate((element) => ({
    width: element.clientWidth,
    contentWidth: element.scrollWidth,
    tops: Array.from(element.children, (child) => child.getBoundingClientRect().top),
  }));
  expect(bounds.contentWidth).toBeLessThanOrEqual(bounds.width);
  expect(new Set(bounds.tops).size).toBe(1);

  await page.setViewportSize({ width: 739, height: 500 });
  await expect(picker).toHaveCount(0);
  await expect(row.getByRole('button')).toHaveCount(6);
  await expect(row.getByRole('button').first()).toHaveText('model-a');
});

test('model picker searches, selects, clears and supports keyboard dismissal', async ({
  page,
}, testInfo) => {
  await page.goto('/stats');
  const picker = page.getByRole('button', {
    name: 'Select Model',
    exact: true,
  });
  await picker.click();
  const dialog = page.getByRole('dialog', { name: 'Select Model' });
  const search = dialog.getByRole('textbox', { name: 'Search', exact: true });
  await expect(search).toBeFocused();
  await page.screenshot({ path: testInfo.outputPath('model-picker.png') });
  await search.fill('no-such-model');
  await expect(dialog.getByText('No matching models found.')).toBeVisible();
  await search.fill('EXAMPLE-MODEL-40');

  const filteredRequest = page.waitForRequest((request) => {
    const url = new URL(request.url());
    return (
      url.pathname === '/api/admin/usage-stats' &&
      url.searchParams.get('model') === 'example-model-40'
    );
  });
  await dialog.getByRole('button', { name: 'example-model-40', exact: true }).click();
  await filteredRequest;
  await expect(dialog).not.toBeVisible();
  await expect(page.getByRole('button', { name: 'example-model-40', exact: true })).toBeVisible();

  await picker.click();
  await expect(search).toHaveValue('');
  await expect(
    dialog.getByRole('button', { name: 'example-model-40', exact: true }),
  ).toHaveAttribute('aria-pressed', 'true');
  await page.keyboard.press('Escape');
  await expect(dialog).not.toBeVisible();
  await expect(picker).toBeFocused();

  await expect(page.getByTestId('stats-model-filter-row').getByRole('button').first()).toHaveText(
    'example-model-40',
  );
  await page
    .getByTestId('stats-model-filter-row')
    .locator('../../..')
    .getByTitle('Clear', { exact: true })
    .click();
  await expect(page.getByRole('button', { name: 'example-model-40', exact: true })).toHaveCount(0);
});
