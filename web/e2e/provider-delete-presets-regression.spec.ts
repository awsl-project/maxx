import { expect, test, type Page, type Route } from '@playwright/test';

const SCREENSHOT_DIR = 'e2e/screenshots/provider-delete-presets';

type ProviderPayload = {
  id: number;
  createdAt: string;
  updatedAt: string;
  name: string;
  type: string;
  supportedClientTypes: string[] | null;
  config: {
    custom?: {
      baseURL?: string;
      apiKey?: string;
      clientBaseURL?: Record<string, string> | null;
    };
  };
};

function nowIso() {
  return new Date().toISOString();
}

function provider(
  id: number,
  name: string,
  overrides: Partial<ProviderPayload> = {},
): ProviderPayload {
  return {
    id,
    createdAt: nowIso(),
    updatedAt: nowIso(),
    name,
    type: 'custom',
    supportedClientTypes: ['openai'],
    config: {
      custom: {
        baseURL: `http://localhost:19999/provider-${id}`,
        apiKey: 'mock-key',
      },
    },
    ...overrides,
  };
}

async function json(route: Route, body: unknown, status = 200) {
  await route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) });
}

async function installCommonMocks(page: Page) {
  await page.addInitScript(() => {
    localStorage.setItem('maxx-admin-token', 'mock-token');
    localStorage.setItem('maxx-ui-language', 'zh');
  });

  await page.route('**/api/settings', (route) =>
    json(route, {
      api_token_auth_enabled: 'true',
      force_project_binding: 'false',
      ui_multitenant_enabled: 'false',
    }),
  );
  await page.route('**/api/admin/settings', (route) =>
    json(route, {
      api_token_auth_enabled: 'true',
      force_project_binding: 'false',
      ui_multitenant_enabled: 'false',
    }),
  );
  await page.route('**/api/admin/auth/status', (route) =>
    json(route, {
      authEnabled: true,
      user: { id: 1, username: 'admin', tenantID: 1, role: 'admin' },
    }),
  );
  await page.route('**/api/proxy-status', (route) =>
    json(route, { running: true, address: '127.0.0.1', port: 9880, version: 'e2e' }),
  );
  await page.route('**/api/admin/proxy-status', (route) =>
    json(route, { running: true, address: '127.0.0.1', port: 9880, version: 'e2e' }),
  );
  await page.route('**/api/admin/provider-stats**', (route) => json(route, []));
  await page.route('**/api/admin/streaming-requests/counts**', (route) => json(route, {}));
  await page.route('**/api/admin/routes**', (route) => json(route, []));
  await page.route('**/api/admin/model-mappings**', (route) => json(route, []));
}

async function screenshot(page: Page, name: string) {
  await page.screenshot({ path: `${SCREENSHOT_DIR}/${name}.png`, fullPage: true });
}

async function selectCustomProvider(page: Page) {
  await page.goto('/providers/create');
  await page.getByRole('button', { name: /Custom|自定义/ }).click();
}

test.use({ viewport: { width: 1440, height: 900 }, locale: 'zh-CN' });

test.describe('Provider delete and preset regression', () => {
  test('quick templates fill Zhipu and DeepSeek compatibility URLs', async ({ page }) => {
    await installCommonMocks(page);

    await selectCustomProvider(page);
    await page.getByRole('button', { name: /智谱|Zhipu/i }).click();
    await expect(page).toHaveURL(/\/providers\/create\/custom/);
    await expect(page.locator('input[value="https://open.bigmodel.cn/api/paas/v4"]')).toBeVisible();
    await expect(page.locator('input[value="https://open.bigmodel.cn/api/anthropic"]')).toHaveCount(
      0,
    );
    await screenshot(page, '01-zhipu-openai-preset-filled');

    await selectCustomProvider(page);
    await page.getByRole('button', { name: /DeepSeek \(OpenAI\)/i }).click();
    await expect(page.locator('input[value="https://api.deepseek.com"]')).toBeVisible();
    await screenshot(page, '02-deepseek-openai-preset-filled');

    await selectCustomProvider(page);
    await page.getByRole('button', { name: /DeepSeek \(Anthropic\)/i }).click();
    await expect(page.locator('input[value="https://api.deepseek.com/anthropic"]')).toBeVisible();
    await screenshot(page, '03-deepseek-anthropic-preset-filled');
  });

  test('sticky edit actions stay reachable when deleting the last provider in a long list', async ({
    page,
  }) => {
    await installCommonMocks(page);
    let providers = Array.from({ length: 18 }, (_, index) =>
      provider(index + 1, `Regression Provider ${String(index + 1).padStart(2, '0')}`),
    );

    await page.route(/\/api\/providers\/?(?:\?.*)?$/, (route) => {
      if (route.request().method() === 'GET') return json(route, providers);
      return route.fallback();
    });
    await page.route('**/api/providers/18', async (route) => {
      if (route.request().method() === 'GET')
        return json(route, provider(18, 'Regression Provider 18'));
      if (route.request().method() === 'DELETE') {
        providers = providers.filter((item) => item.id !== 18);
        return route.fulfill({ status: 204, body: '' });
      }
      return route.fallback();
    });

    await page.goto('/providers');
    await page.getByText('Regression Provider 18').scrollIntoViewIfNeeded();
    await page.getByText('Regression Provider 18').click();
    await expect(page.getByRole('button', { name: /Delete|删除/ }).first()).toBeVisible();
    await screenshot(page, '04-last-provider-sticky-actions-visible-before-delete');

    await page
      .getByRole('button', { name: /Delete|删除/ })
      .first()
      .click();
    await page
      .getByRole('button', { name: /Delete|删除/ })
      .last()
      .click();
    await expect(page).toHaveURL(/\/providers/);
    await expect(page.getByText('Regression Provider 18')).toHaveCount(0);
    await screenshot(page, '05-last-provider-deleted-list-still-stable');
  });

  test('selected-provider bulk delete tolerates nullable result arrays from backend', async ({
    page,
  }) => {
    await installCommonMocks(page);
    await page.route(/\/api\/providers\/?(?:\?.*)?$/, (route) => {
      if (route.request().method() !== 'GET') return route.fallback();
      return json(route, [
        provider(9001, 'Nullable Delete Provider', {
          supportedClientTypes: null,
          config: {
            custom: {
              baseURL: 'http://localhost:19999/nullable-delete',
              apiKey: 'mock-key',
              clientBaseURL: null,
            },
          },
        }),
      ]);
    });
    await page.route('**/api/providers/bulk-delete', (route) =>
      json(route, { deletedCount: 1, deletedIDs: null, notFoundIDs: null }),
    );

    const pageErrors: string[] = [];
    page.on('pageerror', (error) => pageErrors.push(error.message));

    await page.goto('/providers');
    await page.getByRole('checkbox', { name: /Nullable Delete Provider/ }).check();
    await page.getByRole('button', { name: /Delete Selected|删除选中|删除所选|删除已选/ }).click();
    await page
      .getByRole('button', { name: /Delete|删除/ })
      .last()
      .click();
    await expect(page.getByText(/Provider was not deleted|提供商未被删除/i)).toBeVisible();
    await screenshot(page, '06-nullable-bulk-delete-result-no-crash');

    expect(pageErrors).not.toContain("Cannot read properties of null (reading 'filter')");
  });
});
