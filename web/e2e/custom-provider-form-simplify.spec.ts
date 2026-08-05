import { expect, test, type Page, type Route } from '@playwright/test';

const provider = {
  id: 1,
  tenantID: 1,
  name: 'Mock Custom Relay',
  type: 'custom',
  logo: '',
  config: {
    custom: {
      baseURL: 'https://relay.example.test/v1',
      apiKey: 'sk-mock',
      responsesPassthrough: true,
      responsesWebSocket: true,
    },
  },
  supportedClientTypes: ['claude', 'codex'],
  supportModels: ['claude-*'],
  exposedModelsEnabled: true,
  exposedModels: ['claude-sonnet-4-5'],
  maxConcurrency: 3,
  excludeFromExport: false,
  blackBox: false,
};

function json(route: Route, body: unknown) {
  return route.fulfill({ contentType: 'application/json', body: JSON.stringify(body) });
}

async function installProviderMocks(page: Page) {
  await page.addInitScript(() => {
    localStorage.setItem('maxx-admin-token', 'mock-token');
    localStorage.setItem('maxx-ui-language', 'en');
  });

  await page.route('**/api/**', async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname;
    const method = route.request().method();

    if (path === '/api/admin/auth/status') {
      return json(route, {
        authEnabled: true,
        user: { id: 1, username: 'admin', tenantID: 1, role: 'admin' },
      });
    }
    if (path === '/api/admin/settings' || path === '/api/settings') return json(route, {});
    if (path === '/api/admin/proxy-status' || path === '/api/proxy-status') {
      return json(route, { running: true });
    }
    if (path.startsWith('/api/admin/provider-stats')) return json(route, {});
    if (path.includes('/streaming-requests/counts')) return json(route, {});
    if (path === '/api/admin/providers' || path === '/api/providers') {
      if (method === 'POST') return json(route, { ...provider, id: 2 });
      return json(route, [provider]);
    }
    if (path === '/api/providers/1' || path === '/api/admin/providers/1')
      return json(route, provider);
    if (path.includes('/runtime-models')) return json(route, { models: ['claude-sonnet-4-5'] });
    if (
      path === '/api/admin/routes' ||
      path === '/api/routes' ||
      path === '/api/admin/model-mappings' ||
      path === '/api/model-mappings' ||
      path === '/api/admin/projects' ||
      path === '/api/projects' ||
      path === '/api/admin/api-tokens' ||
      path === '/api/api-tokens'
    ) {
      return json(route, []);
    }
    return json(route, method === 'GET' ? {} : { ok: true });
  });
}

test.describe('custom provider form simplification', () => {
  test.beforeEach(async ({ page }) => {
    await installProviderMocks(page);
  });

  test('create flow keeps advanced provider settings collapsed until requested', async ({
    page,
  }) => {
    await page.goto('/providers/create/custom');

    await expect(page.getByRole('heading', { name: /Basic Information|基本信息/i })).toBeVisible();
    await expect(
      page.getByRole('heading', { name: /Client Configuration|客户端配置/i }),
    ).toBeVisible();
    await expect(
      page.getByRole('heading', { name: /Automatic Error Freeze|错误自动冻结/i }),
    ).not.toBeVisible();
    await expect(
      page.getByRole('heading', { name: /Visibility and Export|可见性与导出/i }),
    ).not.toBeVisible();
    await expect(page.getByRole('heading', { name: /Model Mappings|模型映射/i })).not.toBeVisible();

    await page
      .locator('summary')
      .filter({ hasText: /^Advanced settings$/ })
      .click();

    await expect(
      page.getByRole('heading', { name: /Automatic Error Freeze|错误自动冻结/i }),
    ).toBeVisible();
    await expect(
      page.getByRole('heading', { name: /Visibility and Export|可见性与导出/i }),
    ).toBeVisible();
    await expect(page.getByRole('heading', { name: /Model Mappings|模型映射/i })).toBeVisible();
  });

  test('edit flow keeps model and policy sections collapsed until requested', async ({ page }) => {
    await page.goto('/providers/1/edit');

    await expect(page.getByText('Mock Custom Relay').first()).toBeVisible();
    await expect(page.getByRole('heading', { name: /Connection|连接/i })).toBeVisible();
    await expect(page.getByRole('heading', { name: /Client protocols|客户端/i })).toBeVisible();
    await expect(page.locator('#provider-models')).not.toBeVisible();
    await expect(page.locator('#provider-policies')).not.toBeVisible();
    await expect(page.locator('#provider-danger')).not.toBeVisible();

    await page
      .locator('summary')
      .filter({ hasText: /^Advanced settings$/ })
      .click();

    await expect(page.locator('#provider-models')).toBeVisible();
    await expect(page.locator('#provider-policies')).toBeVisible();
    await expect(page.locator('#provider-danger')).toBeVisible();
  });

  test('Gemini Web2API preset opens the custom form with relay-safe defaults', async ({ page }) => {
    await page.goto('/providers/create');

    await page.getByRole('button', { name: /Gemini Web2API/i }).click();

    await expect(page).toHaveURL(/\/providers\/create\/custom/);
    await expect(page.locator('input').first()).toHaveValue('Gemini Web2API');
    await expect(page.locator('input[type="password"]').first()).toHaveValue('sk-gemini');
    await expect(page.getByText(/Do not append \/v1/i)).toBeVisible();
    await expect(page.getByRole('main').getByText('OpenAI', { exact: true })).toBeVisible();
    const switches = await page.locator('[role="switch"]').evaluateAll((elements) =>
      elements.map((element) => ({
        checked: element.getAttribute('aria-checked'),
        text: element.closest('div')?.parentElement?.textContent?.replace(/\s+/g, ' ').trim(),
      })),
    );
    expect(switches).toContainEqual(expect.objectContaining({ checked: 'true', text: 'OpenAI' }));
    expect(switches).toContainEqual(expect.objectContaining({ checked: 'false', text: 'Codex' }));
    expect(switches).toContainEqual(expect.objectContaining({ checked: 'false', text: 'Gemini' }));
    await expect(
      page.getByRole('heading', { name: /Automatic Error Freeze|错误自动冻结/i }),
    ).not.toBeVisible();
  });
});
