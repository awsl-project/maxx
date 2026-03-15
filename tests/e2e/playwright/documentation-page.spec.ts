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

    if (pathname === '/api/admin/api-tokens') {
      return json([
        {
          id: 1,
          name: 'Dev Token',
          tokenPrefix: 'maxx_dev1234567890abcdef...',
          isEnabled: true,
          useCount: 10,
          createdAt: '2026-01-01T00:00:00Z',
          updatedAt: '2026-01-01T00:00:00Z',
        },
        {
          id: 2,
          name: 'Prod Token',
          tokenPrefix: 'maxx_prod678901234abcdef...',
          isEnabled: true,
          useCount: 100,
          createdAt: '2026-01-01T00:00:00Z',
          updatedAt: '2026-01-01T00:00:00Z',
        },
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
  await expect(page.getByTestId('documentation-diagnostics-content')).not.toBeVisible();

  const quickstart = page.getByTestId('documentation-quickstart-content');
  const diagnostics = page.getByTestId('documentation-diagnostics-content');

  const quickstartCodexTab = quickstart.getByRole('tab', { name: 'Codex' });
  await quickstartCodexTab.click();
  await expect(quickstartCodexTab).toHaveAttribute('aria-selected', 'true');

  // Fill token and project slug
  await page.getByTestId('documentation-quickstart-token-input').fill('maxx_docsdemo1234567890abcdef');
  await page.getByTestId('documentation-quickstart-project-slug-input').fill('docs-demo');

  // Generated config should contain the full token from input
  await expect(quickstart).toContainText('maxx_docsdemo1234567890abcdef');

  await page.screenshot({ path: testInfo.outputPath('documentation-quickstart.png'), fullPage: true });

  // Switch to Gemini tab and verify project proxy content
  const quickstartGeminiTab = quickstart.getByRole('tab', { name: 'Gemini' });
  await quickstartGeminiTab.click();
  await expect(quickstartGeminiTab).toHaveAttribute('aria-selected', 'true');
  await expect(quickstart).toContainText('generateContent');

  await page.screenshot({ path: testInfo.outputPath('documentation-quickstart-gemini.png'), fullPage: true });

  // Switch back to Codex and verify state is preserved
  await quickstartCodexTab.click();
  await expect(page.getByTestId('documentation-quickstart-token-input')).toHaveValue(
    'maxx_docsdemo1234567890abcdef',
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

test('token select shows available tokens', async ({ page }) => {
  await page.goto('/documentation');

  // Open the token select dropdown
  await page.getByTestId('documentation-quickstart-token-select').click();
  await expect(page.getByRole('option', { name: /Dev Token/ })).toBeVisible();
  await expect(page.getByRole('option', { name: /Prod Token/ })).toBeVisible();
  await page.keyboard.press('Escape');

  // Fill token input — config should use the full token from input
  await page.getByTestId('documentation-quickstart-token-input').fill('maxx_real_full_token_here');
  await expect(page.getByTestId('documentation-quickstart-content')).toContainText('maxx_real_full_token_here');
});
