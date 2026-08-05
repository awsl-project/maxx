import { expect, test, type Locator, type Page, type Route } from '@playwright/test';

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
  await page.route('**/api/**', async (route) => {
    const url = new URL(route.request().url());
    const path = url.pathname;

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
    if (path === '/api/admin/providers' || path === '/api/providers') return json(route, [provider]);
    if (path === '/api/providers/1' || path === '/api/admin/providers/1') return json(route, provider);
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
    return json(route, {});
  });
}

async function sectionTop(section: Locator) {
  const box = await section.boundingBox();
  expect(box).not.toBeNull();
  return box!.y;
}

function heading(page: Page, name: RegExp) {
  return page.getByRole('heading', { name }).first();
}

async function expectCreateProviderAdvancedSectionsCollapsed(page: Page) {
  const clientConfig = heading(page, /Client Configuration|客户端配置/);
  const advanced = page.locator('summary').filter({ hasText: /^Advanced settings$/ });
  const modelMappings = heading(page, /Model Mappings|模型映射/);
  const errorCooldown = heading(page, /Automatic Error Freeze|错误自动冻结/);
  const visibilityAndExport = heading(page, /Visibility and Export|可见性与导出/);

  await expect(clientConfig).toBeVisible();
  await expect(advanced).toBeVisible();
  await expect(modelMappings).not.toBeVisible();
  await expect(errorCooldown).not.toBeVisible();
  await expect(visibilityAndExport).not.toBeVisible();

  await advanced.click();

  await expect(modelMappings).toBeVisible();
  await expect(errorCooldown).toBeVisible();
  await expect(visibilityAndExport).toBeVisible();
  expect(await sectionTop(errorCooldown)).toBeGreaterThan(await sectionTop(modelMappings));
  expect(await sectionTop(visibilityAndExport)).toBeGreaterThan(await sectionTop(errorCooldown));
}

async function expectEditProviderAdvancedSectionsCollapsed(page: Page) {
  const clients = heading(page, /Client protocols|客户端协议/);
  const advanced = page.locator('summary').filter({ hasText: /^Advanced settings$/ });
  const models = page.locator('#provider-models');
  const policies = page.locator('#provider-policies');
  const danger = page.locator('#provider-danger');

  await expect(clients).toBeVisible();
  await expect(advanced).toBeVisible();
  await expect(models).not.toBeVisible();
  await expect(policies).not.toBeVisible();
  await expect(danger).not.toBeVisible();

  await advanced.click();

  await expect(models).toBeVisible();
  await expect(policies).toBeVisible();
  await expect(danger).toBeVisible();
  expect(await sectionTop(policies)).toBeGreaterThan(await sectionTop(models));
  expect(await sectionTop(danger)).toBeGreaterThan(await sectionTop(policies));
}

test.use({ viewport: { width: 1440, height: 1800 }, locale: 'zh-CN' });

test.describe('Custom provider advanced section layout', () => {
  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem('maxx-admin-token', 'mock-token');
      localStorage.setItem('maxx-ui-language', 'zh');
    });
    await installProviderMocks(page);
  });

  test('create and edit flows keep advanced settings collapsed by default', async ({ page }) => {
    await page.goto('/providers/create/custom');
    await expectCreateProviderAdvancedSectionsCollapsed(page);

    await page.goto('/providers/1/edit');
    await expectEditProviderAdvancedSectionsCollapsed(page);
  });
});
