import { chromium } from '@playwright/test';
import assert from 'node:assert/strict';
import fs from 'node:fs';

const BASE_URL = process.env.BASE_URL || 'http://127.0.0.1:4173';
const CHROME = process.env.CHROME_BIN || '/usr/bin/google-chrome-stable';
const OUT_DIR = process.env.OUT_DIR || '/home/moltbot/clawd-wechat/tmp/maxx-user-panel-tab-state-flow';

fs.mkdirSync(OUT_DIR, { recursive: true });

const MEMBER_A = {
  id: 42,
  username: 'member-a',
  tenantID: 1,
  tenantName: 'Member A',
  role: 'member',
};

const MEMBER_B = {
  id: 43,
  username: 'member-b',
  tenantID: 1,
  tenantName: 'Member B',
  role: 'member',
};

const settings = {
  api_token_auth_enabled: 'true',
  force_project_binding: 'false',
  ui_multitenant_enabled: 'true',
  ui_multitenant_layout: 'user_panel',
};

function nowIso() {
  return new Date().toISOString();
}

function tokenFor(user) {
  return {
    id: 1000 + user.id,
    createdAt: nowIso(),
    updatedAt: nowIso(),
    tenantID: user.tenantID,
    token: '',
    tokenPrefix: `maxx_user_${user.id}_prefix...`,
    name: `User Console Key (user ${user.id})`,
    description: `managed-by=maxx-user-panel;user-id=${user.id}`,
    projectID: 0,
    isEnabled: true,
    devMode: false,
    useCount: 2,
  };
}

function requestsFor(user) {
  return [
    {
      id: user.id * 100,
      requestID: `active-${user.id}`,
      createdAt: nowIso(),
      startTime: nowIso(),
      clientType: 'codex',
      requestModel: 'gpt-5',
      status: 'IN_PROGRESS',
      statusCode: 0,
      duration: 0,
      inputTokenCount: 0,
      outputTokenCount: 0,
      providerID: 1,
      routeID: 1,
      apiTokenID: 1000 + user.id,
      projectID: 0,
    },
    {
      id: user.id * 100 + 1,
      requestID: `done-${user.id}`,
      createdAt: nowIso(),
      startTime: nowIso(),
      clientType: 'openai',
      requestModel: 'gpt-4.1',
      status: 'COMPLETED',
      statusCode: 200,
      duration: 120000000,
      inputTokenCount: 10,
      outputTokenCount: 20,
      providerID: 1,
      routeID: 1,
      apiTokenID: 1000 + user.id,
      projectID: 0,
    },
  ];
}

async function installMock(page, user) {
  await page.route('**/api/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname;
    const method = request.method();
    const json = async (body, status = 200) => {
      await route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) });
    };

    if (path === '/api/settings' || path === '/api/admin/settings') return json(settings);
    if (path === '/api/admin/auth/status') return json({ authEnabled: true, user });
    if (path === '/api/proxy-status' || path === '/api/admin/proxy-status') {
      return json({ running: true, address: '127.0.0.1', port: 9880, version: 'v0.99.0-test' });
    }
    if (path === '/api/user-panel-token' && method === 'GET') {
      return json({ apiToken: tokenFor(user) });
    }
    if (path === '/api/admin/requests') {
      return json({ items: requestsFor(user), hasMore: false, firstId: user.id * 100, lastId: user.id * 100 + 1 });
    }
    if (path === '/api/admin/requests/count') return json(2);
    return json([]);
  });
}

async function preparePage(context, user) {
  const page = await context.newPage();
  await installMock(page, user);
  await page.addInitScript(() => {
    localStorage.setItem('maxx-admin-token', 'mock-token');
    localStorage.setItem('maxx-ui-language', 'zh');
  });
  return page;
}

async function screenshot(page, name) {
  await page.screenshot({ path: `${OUT_DIR}/${name}.png`, fullPage: true });
}

async function assertActiveTab(page, name) {
  await page.getByRole('tab', { name }).waitFor({ state: 'visible', timeout: 10_000 });
  const selected = await page.getByRole('tab', { name }).getAttribute('aria-selected');
  assert.equal(selected, 'true', `${name} tab should be selected`);
}

async function run() {
  const browser = await chromium.launch({
    headless: true,
    executablePath: CHROME,
    args: ['--no-sandbox'],
  });
  const context = await browser.newContext({ viewport: { width: 1440, height: 900 }, locale: 'zh-CN' });

  const userA = await preparePage(context, MEMBER_A);
  await userA.goto(`${BASE_URL}/`);
  await assertActiveTab(userA, '主页');
  await userA.getByRole('tab', { name: /请求/ }).click();
  await assertActiveTab(userA, '请求');
  assert.match(userA.url(), /[?&]tab=requests/, 'clicking requests should update URL tab');
  await userA.reload();
  await assertActiveTab(userA, '请求');
  await userA.getByText('active-42').waitFor({ state: 'visible', timeout: 10_000 });
  await userA.getByRole('tab', { name: /请求/ }).getByText('1', { exact: true }).waitFor({ state: 'visible', timeout: 10_000 });
  await screenshot(userA, '01-user-a-requests-restored-with-badge');

  const userB = await preparePage(context, MEMBER_B);
  await userB.goto(`${BASE_URL}/`);
  await assertActiveTab(userB, '主页');
  await screenshot(userB, '02-user-b-default-home-isolated');

  await userB.getByRole('tab', { name: /请求/ }).click();
  await assertActiveTab(userB, '请求');
  await userB.getByRole('tab', { name: '主页' }).click();
  await assertActiveTab(userB, '主页');
  await userB.reload();
  await assertActiveTab(userB, '主页');
  await screenshot(userB, '03-user-b-home-restored');

  await userA.goto(`${BASE_URL}/`);
  await assertActiveTab(userA, '请求');
  await screenshot(userA, '04-user-a-requests-still-isolated');

  await userA.goto(`${BASE_URL}/?tab=unknown`);
  await assertActiveTab(userA, '请求');
  await screenshot(userA, '05-invalid-url-falls-back-to-user-a-cache');

  await browser.close();
  console.log(JSON.stringify({ ok: true, screenshots: fs.readdirSync(OUT_DIR).sort() }, null, 2));
}

run().catch((error) => {
  console.error(error);
  process.exit(1);
});
