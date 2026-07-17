import { expect, test, type Page, type Route } from '@playwright/test';

async function json(route: Route, body: unknown) {
  await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(body) });
}

type RouteSettings = Record<string, string>;

const now = new Date().toISOString();

function mockApiToken() {
  return {
    apiToken: {
      id: 1,
      createdAt: now,
      updatedAt: now,
      token: '',
      tokenPrefix: 'maxx_mock',
      name: 'User panel token',
      description: '',
      projectID: 0,
      isEnabled: true,
      devMode: false,
      useCount: 0,
    },
  };
}

async function installRouteExposureMocks(
  page: Page,
  settings: RouteSettings,
  role: 'admin' | 'user' = 'admin',
) {
  const hits: Record<string, number> = {};
  const track = async (route: Route, key: string, body: unknown) => {
    hits[key] = (hits[key] || 0) + 1;
    await json(route, body);
  };

  await page.addInitScript(() => {
    localStorage.setItem('maxx-admin-token', 'mock-token');
    localStorage.setItem('maxx-ui-language', 'zh');
  });

  // Playwright resolves the latest matching route first, so register the catch-all before
  // specific routes. Otherwise the fallback masks /api/settings and produces false negatives.
  await page.route('**/api/**', (route) => track(route, 'fallback', {}));
  await page.route(/\/api\/admin\/streaming-requests\/counts(?:\?.*)?$/, (route) =>
    track(route, 'streaming-counts', {}),
  );
  await page.route(/\/api\/admin\/requests\/count(?:\?.*)?$/, (route) =>
    track(route, 'admin-requests-count', 0),
  );
  await page.route(/\/api\/admin\/requests(?:\?.*)?$/, (route) =>
    track(route, 'admin-requests', { items: [], hasMore: false, firstId: null, lastId: null }),
  );
  await page.route('**/api/user-panel-token', (route) => track(route, 'user-panel-token', mockApiToken()));
  await page.route('**/api/routes**', (route) => track(route, 'routes', []));
  await page.route('**/api/providers**', (route) => track(route, 'providers', []));
  await page.route('**/api/proxy-status', (route) =>
    track(route, 'proxy-status', { running: true, address: '127.0.0.1', port: 9880, version: 'e2e' }),
  );
  await page.route('**/api/settings', (route) => track(route, 'public-settings', settings));
  await page.route('**/api/admin/auth/status', (route) =>
    track(route, 'auth-status', {
      authEnabled: true,
      user: { id: 1, username: role === 'admin' ? 'admin' : 'tenant-user', tenantID: 1, role },
    }),
  );

  return hits;
}

async function attachMockEvidence(page: Page, title: string, settings: RouteSettings, hits: Record<string, number>) {
  await page.evaluate(
    ({ title, settings, hits }) => {
      document.querySelector('[data-e2e-mock-evidence]')?.remove();
      const el = document.createElement('div');
      el.dataset.e2eMockEvidence = 'true';
      el.style.cssText = [
        'position:fixed',
        'right:16px',
        'bottom:16px',
        'z-index:99999',
        'max-width:430px',
        'padding:12px',
        'border-radius:12px',
        'background:rgba(10,10,10,0.88)',
        'color:white',
        'font:12px/1.45 ui-monospace,SFMono-Regular,Menlo,monospace',
        'box-shadow:0 12px 40px rgba(0,0,0,0.28)',
        'white-space:pre-wrap',
      ].join(';');
      el.textContent = `${title}\nmock /api/settings = ${JSON.stringify(settings)}\nmock hits = ${JSON.stringify(hits)}`;
      document.body.appendChild(el);
    },
    { title, settings, hits },
  );
}

const defaultSettings: RouteSettings = {
  api_token_auth_enabled: 'false',
  force_project_binding: 'false',
  ui_multitenant_enabled: 'false',
  proxy_route_claude_messages_enabled: 'true',
  proxy_route_openai_chat_enabled: 'true',
  proxy_route_responses_enabled: 'true',
  proxy_route_gemini_enabled: 'false',
};

const filteredSettings: RouteSettings = {
  api_token_auth_enabled: 'false',
  force_project_binding: 'false',
  ui_multitenant_enabled: 'true',
  ui_multitenant_layout: 'user_panel',
  proxy_route_claude_messages_enabled: 'false',
  proxy_route_openai_chat_enabled: 'true',
  proxy_route_responses_enabled: 'false',
  proxy_route_gemini_enabled: 'true',
};

const allOpenAiAndCodexDisabledSettings: RouteSettings = {
  ...filteredSettings,
  proxy_route_openai_chat_enabled: 'false',
  proxy_route_responses_enabled: 'false',
};

test.use({
  locale: 'zh-CN',
  viewport: { width: 1440, height: 1000 },
  launchOptions: { executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || '/usr/bin/google-chrome' },
});

test.describe('route exposure follows public route settings', () => {
  test('documentation quickstart tabs hide disabled client routes', async ({ page }, testInfo) => {
    const defaultHits = await installRouteExposureMocks(page, defaultSettings, 'admin');
    await page.goto('/documentation');
    const tabs = page.getByTestId('documentation-quickstart-client-tabs');
    await expect(tabs).toContainText('Claude');
    await expect(tabs).toContainText('OpenAI');
    await expect(tabs).toContainText('Codex');
    await expect(tabs).not.toContainText('Gemini');
    await attachMockEvidence(page, 'docs default route exposure', defaultSettings, defaultHits);
    await page.screenshot({ path: testInfo.outputPath('01-docs-default-routes.png'), fullPage: true });

    const filteredPage = await page.context().newPage();
    const filteredHits = await installRouteExposureMocks(filteredPage, filteredSettings, 'admin');
    await filteredPage.goto('/documentation');
    const filteredTabs = filteredPage.getByTestId('documentation-quickstart-client-tabs');
    await expect(filteredTabs).not.toContainText('Claude');
    await expect(filteredTabs).toContainText('OpenAI');
    await expect(filteredTabs).not.toContainText('Codex');
    await expect(filteredTabs).toContainText('Gemini');
    await attachMockEvidence(filteredPage, 'docs filtered route exposure', filteredSettings, filteredHits);
    await filteredPage.screenshot({ path: testInfo.outputPath('02-docs-filtered-routes.png'), fullPage: true });
  });

  test('user panel endpoint hints and quickstart follow route exposure settings', async ({ page }, testInfo) => {
    const hits = await installRouteExposureMocks(page, filteredSettings, 'user');
    await page.goto('/');
    await expect(page.getByText('OpenAI / Codex')).toBeVisible();
    await expect(page.getByText('Gemini')).toBeVisible();
    await expect(page.getByText('Claude')).not.toBeVisible();
    await expect(page.getByText('/v1beta/models/{model}:generateContent')).toBeVisible();
    await expect(page.getByText('/v1/chat/completions')).toBeVisible();
    await attachMockEvidence(page, 'user panel filtered route exposure', filteredSettings, hits);
    await page.screenshot({ path: testInfo.outputPath('03-user-panel-filtered-endpoints.png'), fullPage: true });

    const disabledPage = await page.context().newPage();
    const disabledHits = await installRouteExposureMocks(disabledPage, allOpenAiAndCodexDisabledSettings, 'user');
    await disabledPage.goto('/');
    await expect(disabledPage.getByText('OpenAI / Codex')).not.toBeVisible();
    await expect(disabledPage.getByText('/v1/chat/completions')).not.toBeVisible();
    await expect(disabledPage.getByText('Gemini')).toBeVisible();
    await attachMockEvidence(
      disabledPage,
      'user panel OpenAI and Codex disabled',
      allOpenAiAndCodexDisabledSettings,
      disabledHits,
    );
    await disabledPage.screenshot({ path: testInfo.outputPath('04-user-panel-openai-codex-disabled.png'), fullPage: true });
  });
});
