import { chromium } from '@playwright/test';
import assert from 'node:assert/strict';
import fs from 'node:fs';

const BASE_URL = process.env.BASE_URL || 'http://127.0.0.1:4173';
const CHROME = process.env.CHROME_BIN || '/usr/bin/google-chrome-stable';
const OUT_DIR =
  process.env.OUT_DIR || '/home/moltbot/clawd-wechat/tmp/maxx-settings-multitenant-layout';

fs.mkdirSync(OUT_DIR, { recursive: true });

const ADMIN_USER = {
  id: 1,
  username: 'admin',
  tenantID: 1,
  tenantName: 'Admin Tenant',
  role: 'admin',
};

const settings = {
  api_token_auth_enabled: 'true',
  force_project_binding: 'false',
  ui_multitenant_enabled: 'false',
  ui_multitenant_layout: 'current',
};
const calls = [];

function record(method, path, status, note = '') {
  calls.push({ time: new Date().toISOString(), method, path, status, note });
}

async function installMock(page) {
  await page.route('**/api/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname;
    const method = request.method();
    const json = async (body, status = 200, note = '') => {
      record(method, path, status, note);
      await route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) });
    };

    if (path === '/api/admin/auth/status') return json({ authEnabled: true, user: ADMIN_USER });
    if (path === '/api/settings' || path === '/api/admin/settings') return json(settings);
    if (path.startsWith('/api/admin/settings/') && method === 'PUT') {
      const key = decodeURIComponent(path.slice('/api/admin/settings/'.length));
      const body = request.postDataJSON();
      // Simulate a slow network write. The layout selector must appear before this resolves.
      await new Promise((resolve) => setTimeout(resolve, 1200));
      settings[key] = body.value;
      if (key === 'ui_multitenant_enabled') {
        await page.evaluate(() => {
          window.__maxxSettingsSaved = true;
        });
      }
      if (key === 'ui_multitenant_layout') {
        await page.evaluate(() => {
          window.__maxxLayoutSaved = true;
        });
      }
      return json({ key, value: body.value }, 200, `updated ${key}`);
    }
    if (path === '/api/admin/proxy-status') {
      return json({ running: true, address: '127.0.0.1', port: 9880, version: 'v0.99.0-test' });
    }
    if (
      path === '/api/admin/projects' ||
      path === '/api/admin/providers' ||
      path === '/api/admin/routes'
    ) {
      return json([]);
    }
    return json({});
  });
}

async function screenshot(page, name) {
  await page.screenshot({ path: `${OUT_DIR}/${name}.png`, fullPage: true });
}

async function navigateClient(page, path) {
  await page.goto(`${BASE_URL}/`);
  await page.evaluate((targetPath) => {
    window.history.pushState(null, '', targetPath);
    window.dispatchEvent(new PopStateEvent('popstate'));
  }, path);
}

async function run() {
  const browser = await chromium.launch({
    headless: true,
    executablePath: CHROME,
    args: ['--no-sandbox'],
  });
  const context = await browser.newContext({
    viewport: { width: 1440, height: 1000 },
    locale: 'zh-CN',
  });
  const page = await context.newPage();
  await installMock(page);
  await page.addInitScript(() => {
    localStorage.setItem('maxx-admin-token', 'mock-token');
    localStorage.setItem('maxx-ui-language', 'zh');
  });

  await navigateClient(page, '/settings');
  await page
    .getByText('启用多租户界面', { exact: true })
    .waitFor({ state: 'visible', timeout: 10_000 });
  await page
    .getByText('多租户界面式样', { exact: true })
    .waitFor({ state: 'detached', timeout: 10_000 });
  await screenshot(page, '01-before-enable-no-layout-select');

  await page.getByRole('switch', { name: '启用多租户界面' }).click();
  await page
    .getByText('多租户界面式样', { exact: true })
    .waitFor({ state: 'visible', timeout: 500 });
  await screenshot(page, '02-enabled-layout-select-visible-before-save-finishes');
  await page.waitForFunction(() => window.__maxxSettingsSaved === true, null, { timeout: 10_000 });

  await page.getByRole('combobox').filter({ hasText: '当前式样' }).click();
  await page.locator('[data-slot="select-item"]').filter({ hasText: '用户控制台' }).click();
  await page.waitForFunction(() => window.__maxxLayoutSaved === true, null, { timeout: 10_000 });
  await page
    .getByRole('combobox')
    .filter({ hasText: '用户控制台' })
    .waitFor({ state: 'visible', timeout: 10_000 });
  await screenshot(page, '03-layout-changed-user-panel');

  assert.equal(settings.ui_multitenant_enabled, 'true');
  assert.equal(settings.ui_multitenant_layout, 'user_panel');
  assert(calls.some((call) => call.note === 'updated ui_multitenant_enabled'));
  assert(calls.some((call) => call.note === 'updated ui_multitenant_layout'));

  const ledger = await context.newPage();
  await ledger.setContent(`
    <html lang="zh-CN">
      <head><meta charset="utf-8"><style>
        body { font-family: Inter, system-ui, sans-serif; padding: 24px; background: #f8fafc; color: #0f172a; }
        table { width: 100%; border-collapse: collapse; background: white; }
        th, td { border: 1px solid #cbd5e1; padding: 8px; font-size: 13px; text-align: left; }
        th { background: #e2e8f0; }
        code { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }
      </style></head>
      <body>
        <h1>设置页 Mock 交互证据</h1>
        <p>多租户开关点击后，式样下拉在保存请求返回前已出现。</p>
        <p>最终设置：<code>${JSON.stringify(settings)}</code></p>
        <table>
          <thead><tr><th>时间</th><th>方法</th><th>路径</th><th>状态</th><th>说明</th></tr></thead>
          <tbody>${calls
            .map(
              (call) =>
                `<tr><td>${call.time}</td><td>${call.method}</td><td><code>${call.path}</code></td><td>${call.status}</td><td>${call.note}</td></tr>`,
            )
            .join('')}</tbody>
        </table>
      </body>
    </html>
  `);
  await ledger.screenshot({ path: `${OUT_DIR}/04-mock-interaction-ledger.png`, fullPage: true });

  await browser.close();
  console.log(
    JSON.stringify({ ok: true, screenshots: fs.readdirSync(OUT_DIR).sort(), calls }, null, 2),
  );
}

run().catch((error) => {
  console.error(error);
  process.exit(1);
});
