import { chromium } from '@playwright/test';
import assert from 'node:assert/strict';
import fs from 'node:fs';

const BASE_URL = process.env.BASE_URL || 'http://127.0.0.1:4173';
const CHROME = process.env.CHROME_BIN || '/usr/bin/google-chrome-stable';
const OUT_DIR = process.env.OUT_DIR || '/home/moltbot/clawd-wechat/tmp/maxx-user-panel-key-flow';

fs.mkdirSync(OUT_DIR, { recursive: true });

const ADMIN_USER = {
  id: 1,
  username: 'admin',
  tenantID: 1,
  tenantName: 'Admin Tenant',
  role: 'admin',
};

const MEMBER_USER = {
  id: 42,
  username: 'member-user',
  tenantID: 1,
  tenantName: 'Member Workspace',
  role: 'member',
};

const state = {
  nextTokenIndex: 1,
  tokens: [],
  calls: [],
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

function userPanelMarker(userId) {
  return `managed-by=maxx-user-panel;user-id=${userId}`;
}

function sanitize(token) {
  if (!token) return null;
  return { ...token, token: '' };
}

function createUserPanelToken(userId) {
  const index = state.nextTokenIndex++;
  const plain = `maxx_user_panel_${index}_abcdefghijklmnopqrstuvwxyz0123456789`;
  const token = {
    id: 100 + index,
    createdAt: nowIso(),
    updatedAt: nowIso(),
    tenantID: 1,
    token: plain,
    tokenPrefix: `${plain.slice(0, 24)}...`,
    name: `User Console Key (user ${userId})`,
    description: userPanelMarker(userId),
    projectID: 0,
    isEnabled: true,
    devMode: false,
    useCount: index === 1 ? 0 : 0,
  };
  state.tokens.unshift(token);
  return { token: plain, apiToken: sanitize(token) };
}

function currentUserPanelToken(userId) {
  return state.tokens.find((token) => token.description === userPanelMarker(userId)) || null;
}

function removeUserPanelTokens(userId) {
  const marker = userPanelMarker(userId);
  const removed = state.tokens.filter((token) => token.description === marker);
  state.tokens = state.tokens.filter((token) => token.description !== marker);
  return removed;
}

function record(method, path, status, note = '') {
  state.calls.push({ time: new Date().toISOString(), method, path, status, note });
}

async function installMock(page, user) {
  await page.route('**/api/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname;
    const method = request.method();
    const json = async (body, status = 200, note = '') => {
      record(method, path, status, note);
      await route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) });
    };

    if (path === '/api/settings' || path === '/api/admin/settings') return json(settings);
    if (path === '/api/admin/auth/status') return json({ authEnabled: true, user });
    if (path === '/api/proxy-status' || path === '/api/admin/proxy-status') {
      return json({ running: true, address: '127.0.0.1', port: 9880, version: 'v0.99.0-test' });
    }
    if (path === '/api/usage-stats') {
      return json([
        {
          id: 1,
          createdAt: nowIso(),
          timeBucket: nowIso(),
          granularity: 'hour',
          routeID: 1,
          providerID: 1,
          projectID: 0,
          apiTokenID: currentUserPanelToken(MEMBER_USER.id)?.id || 0,
          clientType: 'openai',
          model: 'gpt-test',
          totalRequests: 25,
          successfulRequests: 24,
          failedRequests: 1,
          totalDurationMs: 1200,
          totalTtftMs: 300,
          inputTokens: 1000,
          outputTokens: 500,
          cacheRead: 0,
          cacheWrite: 0,
          cost: 1200,
        },
      ]);
    }
    if (path === '/api/user-panel-token' && method === 'GET') {
      return json(
        { apiToken: sanitize(currentUserPanelToken(user.id)) },
        200,
        'read dedicated token',
      );
    }
    if (path === '/api/user-panel-token' && method === 'POST') {
      const existing = currentUserPanelToken(user.id);
      if (existing)
        return json(
          { error: 'user panel token already exists', apiToken: sanitize(existing) },
          409,
          'duplicate create blocked',
        );
      return json(createUserPanelToken(user.id), 201, 'create dedicated token');
    }
    if (path === '/api/user-panel-token/regenerate' && method === 'POST') {
      const removed = removeUserPanelTokens(user.id);
      const created = createUserPanelToken(user.id);
      return json(
        created,
        201,
        `regenerated; removed=${removed.map((token) => token.id).join(',')}`,
      );
    }
    if (path === '/api/api-tokens') return json(state.tokens.map(sanitize));
    if (path === '/api/admin/api-tokens') return json(state.tokens);
    if (path === '/api/admin/projects' || path === '/api/projects') return json([]);
    if (path === '/api/admin/requests/count') return json(0);
    if (path === '/api/admin/requests')
      return json({ items: [], hasMore: false, firstId: 0, lastId: 0 });
    if (
      [
        '/api/admin/providers',
        '/api/providers',
        '/api/admin/routes',
        '/api/routes',
        '/api/admin/model-mappings',
        '/api/model-mappings',
        '/api/admin/response-models',
        '/api/response-models',
        '/api/admin/sessions',
        '/api/sessions',
      ].includes(path)
    ) {
      return json([]);
    }

    return json({});
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

async function assertVisible(page, text) {
  await page.getByText(text, { exact: true }).waitFor({ state: 'visible', timeout: 10_000 });
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
    viewport: { width: 1440, height: 900 },
    locale: 'zh-CN',
  });
  await context.grantPermissions(['clipboard-read', 'clipboard-write'], { origin: BASE_URL });

  const userPage = await preparePage(context, MEMBER_USER);
  await userPage.goto(`${BASE_URL}/`);
  await assertVisible(userPage, '暂无专用 Key');
  await screenshot(userPage, '01-user-no-key');

  await userPage.getByRole('button', { name: /创建 Key/ }).click();
  await assertVisible(userPage, '请立即复制');
  assert.equal(state.tokens.length, 1, 'create should add exactly one admin-visible token');
  const firstPlain = state.tokens[0].token;
  await userPage
    .getByText(firstPlain, { exact: true })
    .waitFor({ state: 'visible', timeout: 10_000 });
  await screenshot(userPage, '02-user-created-one-time-key');

  await userPage.getByRole('button', { name: /复制 Key/ }).click();
  const copied = await userPage.evaluate(() => navigator.clipboard.readText());
  assert.equal(copied, firstPlain, 'copy button should copy the one-time full key');

  const duplicateStatus = await userPage.evaluate(async () => {
    const response = await fetch('/api/user-panel-token', { method: 'POST' });
    return response.status;
  });
  assert.equal(duplicateStatus, 409, 'second create without regenerate should be blocked');

  await userPage.reload();
  await assertVisible(userPage, '完整 Key 已隐藏，可重新生成。');
  await userPage
    .getByText(firstPlain, { exact: true })
    .waitFor({ state: 'detached', timeout: 10_000 })
    .catch(() => {
      throw new Error('full key must not be visible after reload');
    });
  await screenshot(userPage, '03-user-reload-full-key-hidden');

  const adminPage = await preparePage(context, ADMIN_USER);
  await navigateClient(adminPage, '/api-tokens');
  await adminPage
    .getByText('User Console Key (user 42)', { exact: false })
    .waitFor({ state: 'visible', timeout: 10_000 });
  await screenshot(adminPage, '04-admin-token-visible-after-create');

  userPage.once('dialog', (dialog) => dialog.accept());
  await userPage.getByRole('button', { name: /重新生成/ }).click();
  await assertVisible(userPage, '请立即复制');
  assert.equal(state.tokens.length, 1, 'regenerate should replace old token with one token');
  const secondPlain = state.tokens[0].token;
  assert.notEqual(secondPlain, firstPlain, 'regenerated token should differ from old token');
  await userPage
    .getByText(secondPlain, { exact: true })
    .waitFor({ state: 'visible', timeout: 10_000 });
  assert.equal(state.tokens[0].id, 102, 'new token should be the only admin-visible token');
  await screenshot(userPage, '05-user-regenerated-one-time-key');

  await navigateClient(adminPage, '/api-tokens');
  await adminPage
    .getByText('User Console Key (user 42)', { exact: false })
    .waitFor({ state: 'visible', timeout: 10_000 });
  await screenshot(adminPage, '06-admin-token-visible-after-regenerate');

  const ledger = await context.newPage();
  await ledger.setContent(`
    <html lang="zh-CN">
      <head><meta charset="utf-8"><style>
        body { font-family: Inter, system-ui, sans-serif; padding: 24px; background: #f8fafc; color: #0f172a; }
        h1 { margin-top: 0; }
        table { width: 100%; border-collapse: collapse; background: white; }
        th, td { border: 1px solid #cbd5e1; padding: 8px; font-size: 13px; text-align: left; }
        th { background: #e2e8f0; }
        code { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }
      </style></head>
      <body>
        <h1>Mock 交互证据</h1>
        <p>当前 admin 可见 Token 数：<strong>${state.tokens.length}</strong></p>
        <p>当前 Token：<code>${state.tokens[0].tokenPrefix}</code>，完整值仅在用户创建/重新生成瞬间出现。</p>
        <table>
          <thead><tr><th>时间</th><th>方法</th><th>路径</th><th>状态</th><th>说明</th></tr></thead>
          <tbody>${state.calls
            .map(
              (call) =>
                `<tr><td>${call.time}</td><td>${call.method}</td><td><code>${call.path}</code></td><td>${call.status}</td><td>${call.note}</td></tr>`,
            )
            .join('')}</tbody>
        </table>
      </body>
    </html>
  `);
  await screenshot(ledger, '07-mock-interaction-ledger');

  await browser.close();
  console.log(
    JSON.stringify(
      { ok: true, screenshots: fs.readdirSync(OUT_DIR).sort(), calls: state.calls.length },
      null,
      2,
    ),
  );
}

run().catch((error) => {
  console.error(error);
  process.exit(1);
});
