import { chromium } from '@playwright/test';
import assert from 'node:assert/strict';
import fs from 'node:fs';

const BASE_URL = process.env.BASE_URL || 'http://127.0.0.1:4173';
const CHROME = process.env.CHROME_BIN || '/usr/bin/google-chrome-stable';
const OUT_DIR = process.env.OUT_DIR || '/home/moltbot/clawd-wechat/tmp/maxx-multitenant-login';

fs.mkdirSync(OUT_DIR, { recursive: true });

const calls = [];
const settings = {
  api_token_auth_enabled: 'true',
  force_project_binding: 'false',
  ui_multitenant_enabled: 'true',
  ui_multitenant_layout: 'user_panel',
};

async function installMock(page) {
  await page.route('**/api/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname;
    const method = request.method();
    const json = async (body, status = 200) => {
      await route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) });
    };

    if (path === '/api/settings' || path === '/api/admin/settings') return json(settings);
    if (path === '/api/admin/auth/status') return json({ authEnabled: true });
    if (path === '/api/admin/auth/login') {
      const body = request.postDataJSON();
      calls.push({ method, path, body });
      return json({ success: false, error: 'invalid credentials' }, 401);
    }
    if (path === '/api/admin/auth/passkey/login/options') {
      return json({ success: false, error: 'not configured' });
    }
    if (path === '/api/admin/auth/apply') {
      return json({ success: false, error: 'invalid invite code' }, 400);
    }
    return json({ error: `unexpected mock path: ${path}` }, 404);
  });
}

async function assertVisible(page, text) {
  await page.getByText(text, { exact: true }).waitFor({ timeout: 10_000 });
}

async function screenshot(page, name) {
  await page.screenshot({ path: `${OUT_DIR}/${name}.png`, fullPage: true });
}

async function run() {
  const browser = await chromium.launch({ executablePath: CHROME, headless: true });
  const context = await browser.newContext({
    viewport: { width: 1280, height: 900 },
    deviceScaleFactor: 1,
    locale: 'zh-CN',
  });
  await context.addInitScript(() => localStorage.setItem('maxx-ui-language', 'zh'));
  const page = await context.newPage();
  await installMock(page);

  await page.goto(BASE_URL, { waitUntil: 'networkidle' });
  await page.getByRole('heading', { name: 'Maxx 管理后台' }).waitFor({ timeout: 10_000 });
  const accountTab = page.getByRole('tab', { name: '账号登录' });
  const adminTab = page.getByRole('tab', { name: '管理员登录' });
  const registerTab = page.getByRole('tab', { name: '申请新账号' });
  await accountTab.waitFor({ timeout: 10_000 });
  await adminTab.waitFor({ timeout: 10_000 });
  await registerTab.waitFor({ timeout: 10_000 });
  await accountTab.evaluate((node) => {
    if (!node.hasAttribute('data-active')) throw new Error('account tab should be visibly active');
  });
  await page.getByLabel('用户名').waitFor({ timeout: 10_000 });
  await page.getByLabel('密码').waitFor({ timeout: 10_000 });
  await page.getByText('连接设置', { exact: true }).waitFor({ timeout: 10_000 });
  await screenshot(page, '01-multitenant-account-login');

  await adminTab.click();
  await adminTab.evaluate((node) => {
    if (!node.hasAttribute('data-active')) throw new Error('admin tab should be visibly active');
  });
  await page.locator('#login-username').waitFor({ state: 'detached', timeout: 10_000 });
  await page
    .getByText('Passkey 登录', { exact: true })
    .waitFor({ state: 'detached', timeout: 10_000 });
  await page.getByText('连接设置', { exact: true }).waitFor({ timeout: 10_000 });
  await page.getByLabel('密码').waitFor({ timeout: 10_000 });
  await screenshot(page, '02-multitenant-admin-login');
  await page.getByLabel('密码').fill('wrong-password');
  await page.getByRole('button', { name: '登录' }).click();
  await assertVisible(page, '用户名或密码错误');
  assert.deepEqual(calls.at(-1), {
    method: 'POST',
    path: '/api/admin/auth/login',
    body: { username: 'admin', password: 'wrong-password' },
  });

  await registerTab.click();
  await registerTab.evaluate((node) => {
    if (!node.hasAttribute('data-active')) throw new Error('register tab should be visibly active');
  });
  await page.getByLabel('邀请码').waitFor({ timeout: 10_000 });
  await page.getByText('连接设置', { exact: true }).waitFor({ timeout: 10_000 });
  await screenshot(page, '03-multitenant-register');

  await browser.close();
  console.log(
    JSON.stringify({ ok: true, screenshots: fs.readdirSync(OUT_DIR).sort(), calls }, null, 2),
  );
}

run().catch((error) => {
  console.error(error);
  process.exit(1);
});
