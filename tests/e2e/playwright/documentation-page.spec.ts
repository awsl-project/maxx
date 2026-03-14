import { expect, test, type Page } from 'playwright/test';

async function mockDocumentationApis(page: Page) {
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
      return json({ api_token_auth_enabled: 'true' });
    }

    if (pathname === '/api/admin/proxy-status') {
      return json({ running: true, address: '127.0.0.1', port: 9880, version: 'v0.12.31' });
    }

    if (pathname === '/api/admin/providers') {
      return json([
        { id: 1, name: 'Claude Pool', type: 'claude' },
        { id: 2, name: 'Codex Pool', type: 'codex' },
      ]);
    }

    if (pathname === '/api/admin/routes') {
      return json([
        { id: 1, name: 'Default Route', isEnabled: true },
        { id: 2, name: 'Disabled Route', isEnabled: false },
      ]);
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
  await mockDocumentationApis(page);
});

test('documentation page keeps tab state and links quick start to diagnostics', async ({ page }, testInfo) => {
  await page.goto('/documentation');

  await expect(page.getByTestId('documentation-page-tabs')).toBeVisible();
  await expect(page.getByTestId('documentation-quickstart-content')).toBeVisible();
  await expect(page.getByTestId('documentation-examples-content')).not.toBeVisible();
  await expect(page.getByTestId('documentation-diagnostics-content')).not.toBeVisible();

  const quickstart = page.getByTestId('documentation-quickstart-content');
  const examples = page.getByTestId('documentation-examples-content');
  const diagnostics = page.getByTestId('documentation-diagnostics-content');

  const quickstartCodexTab = quickstart.getByRole('tab', { name: 'Codex' });
  await quickstartCodexTab.click();
  await expect(quickstartCodexTab).toHaveAttribute('aria-selected', 'true');

  await page.getByTestId('documentation-quickstart-token-input').fill('maxx_docsdemo12345');
  await page.getByTestId('documentation-quickstart-project-slug-input').fill('docs-demo');

  await page.screenshot({ path: testInfo.outputPath('documentation-quickstart.png'), fullPage: true });

  await page.getByTestId('documentation-page-tab-examples').click();
  await expect(examples).toBeVisible();

  const examplesGeminiTab = examples.getByRole('tab', { name: 'Gemini' });
  await examplesGeminiTab.click();
  await expect(examplesGeminiTab).toHaveAttribute('aria-selected', 'true');
  await expect(examples).toContainText('generateContent');

  await page.screenshot({ path: testInfo.outputPath('documentation-examples.png'), fullPage: true });

  await page.getByTestId('documentation-page-tab-quickstart').click();
  await expect(quickstart).toBeVisible();
  await expect(page.getByTestId('documentation-quickstart-token-input')).toHaveValue(
    'maxx_docsdemo12345',
  );
  await expect(page.getByTestId('documentation-quickstart-project-slug-input')).toHaveValue(
    'docs-demo',
  );
  await expect(quickstartCodexTab).toHaveAttribute('aria-selected', 'true');

  await page.getByTestId('documentation-open-diagnostics-button').click();
  await expect(diagnostics).toBeVisible();
  await expect(page.getByTestId('documentation-page-tab-diagnostics')).toHaveAttribute(
    'aria-selected',
    'true',
  );
  await expect(page.getByTestId('documentation-diagnostics-list').locator(':scope > *')).toHaveCount(
    5,
  );
  await expect(diagnostics.getByText(/^(Action Needed|待处理)$/)).toHaveCount(0);

  await page.screenshot({ path: testInfo.outputPath('documentation-diagnostics.png'), fullPage: true });
});
