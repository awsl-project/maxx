import { expect, test, type Page, type Route } from 'playwright/test';

const ADMIN_USER = {
  id: 1,
  username: 'admin',
  tenantID: 1,
  tenantName: 'Admin Tenant',
  role: 'admin',
};

function json(route: Route, body: unknown, status = 200) {
  return route.fulfill({
    status,
    contentType: 'application/json',
    body: JSON.stringify(body),
  });
}

async function mockCommonApis(page: Page, settingsGate: Promise<void>) {
  await page.route('**/api/**', async (route) => {
    const url = new URL(route.request().url());
    const { pathname } = url;

    if (pathname === '/api/settings') {
      await settingsGate;
      return json(route, {
        force_project_binding: 'false',
        ui_multitenant_enabled: 'false',
      });
    }

    if (pathname === '/api/admin/auth/status') {
      const token = await route.request().headerValue('authorization');
      return json(route, {
        authEnabled: true,
        user: token ? ADMIN_USER : null,
      });
    }

    if (pathname === '/api/admin/proxy-status' || pathname === '/api/proxy-status') {
      return json(route, {
        running: true,
        address: '127.0.0.1',
        port: 9880,
        version: 'v0.13.61',
      });
    }

    if (
      pathname === '/api/admin/providers' ||
      pathname === '/api/providers' ||
      pathname === '/api/admin/routes' ||
      pathname === '/api/routes' ||
      pathname === '/api/admin/projects' ||
      pathname === '/api/projects' ||
      pathname === '/api/admin/api-tokens' ||
      pathname === '/api/api-tokens' ||
      pathname === '/api/admin/model-mappings' ||
      pathname === '/api/model-mappings' ||
      pathname === '/api/admin/response-models' ||
      pathname === '/api/response-models' ||
      pathname === '/api/admin/sessions' ||
      pathname === '/api/sessions'
    ) {
      return json(route, []);
    }

    if (pathname === '/api/admin/requests/count') {
      return json(route, 0);
    }

    if (pathname === '/api/admin/requests') {
      return json(route, { items: [], hasMore: false, firstId: 0, lastId: 0 });
    }

    return json(route, {});
  });
}

test('login does not flash multi-tenant controls while disabled setting is loading', async ({
  page,
}) => {
  let resolveSettings!: () => void;
  const settingsGate = new Promise<void>((resolve) => {
    resolveSettings = resolve;
  });
  await mockCommonApis(page, settingsGate);

  await page.goto('/', { waitUntil: 'domcontentloaded' });

  await expect(page.getByLabel(/password/i)).toBeVisible();
  await expect(page.locator('input[type="text"]')).toHaveCount(0);
  await expect(page.getByRole('tab', { name: /register/i })).toHaveCount(0);
  await expect(page.getByText(/passkey/i)).toHaveCount(0);
  await expect(page.getByRole('button', { name: /sign in|login/i })).toBeDisabled();

  resolveSettings();

  await expect(page.locator('input[type="text"]')).toHaveCount(0);
  await expect(page.getByRole('tab', { name: /register/i })).toHaveCount(0);
  await expect(page.getByText(/passkey/i)).toHaveCount(0);
  await expect(page.getByRole('button', { name: /sign in|login/i })).toBeDisabled();
});

test('sidebar does not flash multi-tenant navigation while disabled setting is loading', async ({
  page,
}) => {
  let resolveSettings!: () => void;
  const settingsGate = new Promise<void>((resolve) => {
    resolveSettings = resolve;
  });
  await mockCommonApis(page, settingsGate);
  await page.addInitScript(() => localStorage.setItem('maxx-admin-token', 'admin-token'));

  await page.goto('/', { waitUntil: 'domcontentloaded' });

  await expect(page.getByText('ROUTES')).toHaveCount(0);
  await expect(page.getByRole('link', { name: /invite codes/i })).toHaveCount(0);
  await expect(page.getByRole('link', { name: /^users$/i })).toHaveCount(0);
  await expect(page.getByRole('link', { name: /claude/i })).toHaveCount(0);

  resolveSettings();

  await expect(page.getByText('ROUTES')).toHaveCount(0);
  await expect(page.getByRole('link', { name: /invite codes/i })).toHaveCount(0);
  await expect(page.getByRole('link', { name: /^users$/i })).toHaveCount(0);
  await expect(page.getByRole('link', { name: /claude/i })).toHaveCount(0);
});
