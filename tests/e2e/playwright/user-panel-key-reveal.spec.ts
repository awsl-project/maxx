import http from 'node:http';

import { expect, test } from 'playwright/test';

import { BASE, adminAPI, closeServer, loginToAdminAPI } from './helpers';

test.describe.configure({ mode: 'serial' });

function startMockOpenAIServer(): Promise<{ server: http.Server; port: number; requests: string[] }> {
  const requests: string[] = [];

  return new Promise((resolve) => {
    const server = http.createServer((req, res) => {
      if (req.method === 'POST' && req.url?.includes('/v1/chat/completions')) {
        let body = '';
        req.on('data', (chunk) => {
          body += chunk;
        });
        req.on('end', () => {
          requests.push(body);

          let parsed: any = {};
          try {
            parsed = JSON.parse(body);
          } catch {
            // keep mock permissive; malformed bodies still get captured above
          }

          res.writeHead(200, { 'Content-Type': 'application/json' });
          res.end(
            JSON.stringify({
              id: `chatcmpl_key_reveal_${Date.now()}`,
              object: 'chat.completion',
              created: Math.floor(Date.now() / 1000),
              model: parsed.model || 'gpt-4o-mini',
              choices: [
                {
                  index: 0,
                  message: { role: 'assistant', content: 'mock key reveal proxy ok' },
                  finish_reason: 'stop',
                },
              ],
              usage: { prompt_tokens: 9, completion_tokens: 6, total_tokens: 15 },
            }),
          );
        });
        return;
      }

      res.writeHead(404, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ error: 'not found' }));
    });

    server.listen(0, '127.0.0.1', () => {
      const address = server.address();
      if (!address || typeof address === 'string') {
        throw new Error('Failed to determine mock server port');
      }
      resolve({ server, port: address.port, requests });
    });
  });
}

async function sendOpenAIProxyRequest(apiToken: string, marker: string) {
  const response = await fetch(`${BASE}/v1/chat/completions`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${apiToken}`,
    },
    body: JSON.stringify({
      model: 'gpt-4o-mini',
      messages: [{ role: 'user', content: marker }],
      max_tokens: 32,
    }),
  });

  const text = await response.text();
  if (!response.ok) {
    throw new Error(`Proxy request failed (${response.status}): ${text}`);
  }
  return JSON.parse(text);
}

async function loginViaUI(page: import('playwright/test').Page, username: string, password: string) {
  await page.goto(BASE);
  await page.locator('input[type="text"]').fill(username);
  await page.locator('input[type="password"]').fill(password);
  await page.locator('button[type="submit"]').click();
  await expect(page.locator('body')).toContainText(/User Console|用户控制台/, { timeout: 15000 });
}

async function memberAPI(method: string, path: string, token: string, body?: unknown): Promise<any> {
  const response = await fetch(`${BASE}/api${path}`, {
    method,
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${token}`,
    },
    body: body ? JSON.stringify(body) : undefined,
  });
  const text = await response.text();
  let json: any = text;
  try {
    json = JSON.parse(text);
  } catch {
    // keep text for error reporting
  }
  if (!response.ok) {
    throw new Error(`Member API ${method} ${path} failed (${response.status}): ${text}`);
  }
  return json;
}

test('user panel key reveal uses a controlled endpoint and eye-toggle UI', async ({ page }, testInfo) => {
  const adminToken = await loginToAdminAPI();
  const username = `member-key-reveal-${Date.now()}`;
  const password = 'Member123!';
  const mock = await startMockOpenAIServer();
  let createdUserId: number | null = null;
  let providerId: number | null = null;
  let routeId: number | null = null;
  const previousSettings = await adminAPI('GET', '/settings', undefined, adminToken);

  try {
    await adminAPI('PUT', '/settings/ui_multitenant_enabled', { value: 'true' }, adminToken);
    await adminAPI('PUT', '/settings/ui_multitenant_layout', { value: 'user_panel' }, adminToken);

    const provider = await adminAPI(
      'POST',
      '/providers',
      {
        name: `Key Reveal Mock OpenAI ${Date.now()}`,
        type: 'custom',
        config: {
          custom: {
            baseURL: `http://127.0.0.1:${mock.port}`,
            apiKey: 'mock-provider-key',
          },
        },
        supportedClientTypes: ['openai'],
        supportModels: ['*'],
      },
      adminToken,
    );
    providerId = provider.id;

    const route = await adminAPI(
      'POST',
      '/routes',
      {
        isEnabled: true,
        isNative: false,
        clientType: 'openai',
        providerID: provider.id,
        projectID: 0,
        position: 1,
      },
      adminToken,
    );
    routeId = route.id;

    const user = await adminAPI(
      'POST',
      '/users',
      {
        username,
        password,
        role: 'member',
      },
      adminToken,
    );
    createdUserId = user.id;

    await loginViaUI(page, username, password);
    await expect(page.getByText(/Generate|生成/)).toBeVisible({ timeout: 10000 });

    await page.getByRole('button', { name: /Generate|生成/ }).click();
    await expect(page.getByText(/Shown once|仅显示一次/)).toBeVisible({ timeout: 10000 });
    const createdBodyText = (await page.textContent('body')) ?? '';
    const createdFullKey = createdBodyText.match(/maxx_[0-9a-f]{64}/)?.[0] ?? '';
    expect(createdFullKey).toMatch(/^maxx_[0-9a-f]{64}$/);

    const createdScreenshot = '/tmp/maxx-user-panel-key-created-visible.png';
    await page.screenshot({ path: createdScreenshot, fullPage: true });
    await testInfo.attach('user-panel-key-created-visible', {
      path: createdScreenshot,
      contentType: 'image/png',
    });

    const memberLogin = await adminAPI('POST', '/auth/login', { username, password });
    const memberToken = memberLogin.token as string;
    expect(memberToken).toBeTruthy();

    const tokenMetadata = await memberAPI('GET', '/user-panel-token', memberToken);
    expect(tokenMetadata.apiToken.token).toBe('');
    expect(tokenMetadata.apiToken.tokenPrefix).toMatch(/^maxx_/);

    await page.reload();
    await expect(page.locator('body')).toContainText(/User Console|用户控制台/, { timeout: 15000 });
    const reloadedKeyInput = page.getByLabel(/Key|密钥/).first();
    await expect(reloadedKeyInput).toHaveAttribute('type', 'password');
    await expect(reloadedKeyInput).not.toHaveValue(createdFullKey);

    const hiddenScreenshot = '/tmp/maxx-user-panel-key-hidden-after-reload.png';
    await page.screenshot({ path: hiddenScreenshot, fullPage: true });
    await testInfo.attach('user-panel-key-hidden-after-reload', {
      path: hiddenScreenshot,
      contentType: 'image/png',
    });

    const revealed = await memberAPI('POST', '/user-panel-token/reveal', memberToken);
    expect(revealed.token).toBe(createdFullKey);

    await page.getByRole('button', { name: /Show key|显示密钥/ }).click();
    await expect(reloadedKeyInput).toHaveAttribute('type', 'text');
    await expect(reloadedKeyInput).toHaveValue(createdFullKey);
    await expect(page.getByText(/Full key is visible|完整密钥已显示/)).toBeVisible();

    const revealedScreenshot = '/tmp/maxx-user-panel-key-revealed-by-eye.png';
    await page.screenshot({ path: revealedScreenshot, fullPage: true });
    await testInfo.attach('user-panel-key-revealed-by-eye', {
      path: revealedScreenshot,
      contentType: 'image/png',
    });

    await page.getByRole('button', { name: /Hide key|隐藏密钥/ }).click();
    await expect(reloadedKeyInput).toHaveAttribute('type', 'password');
    await expect(reloadedKeyInput).not.toHaveValue(createdFullKey);

    const marker = `key reveal proxy marker ${Date.now()}`;
    const proxyResponse = await sendOpenAIProxyRequest(revealed.token, marker);
    expect(proxyResponse.choices?.[0]?.message?.content).toBe('mock key reveal proxy ok');
    expect(mock.requests).toHaveLength(1);
    expect(mock.requests[0]).toContain(marker);

    const mockFlowScreenshot = '/tmp/maxx-user-panel-key-mock-proxy-response.png';
    await page.evaluate(
      ({ markerText, responseText, mockCount }) => {
        const panel = document.createElement('section');
        panel.setAttribute('data-testid', 'mock-proxy-evidence');
        panel.style.cssText =
          'position:fixed;left:24px;right:24px;bottom:24px;z-index:9999;padding:16px;border:2px solid #16a34a;border-radius:12px;background:white;color:#111;font:13px ui-monospace,monospace;box-shadow:0 12px 30px rgba(0,0,0,.18)';
        panel.textContent = `Mock OpenAI received ${mockCount} request(s); marker=${markerText}; response=${responseText}`;
        document.body.appendChild(panel);
      },
      { markerText: marker, responseText: proxyResponse.choices[0].message.content, mockCount: mock.requests.length },
    );
    await page.screenshot({ path: mockFlowScreenshot, fullPage: true });
    await testInfo.attach('user-panel-key-mock-proxy-response', {
      path: mockFlowScreenshot,
      contentType: 'image/png',
    });

    await page.getByRole('tab', { name: /Requests|请求记录/ }).click();
    await expect(page.locator('body')).toContainText('gpt-4o-mini', { timeout: 15000 });
    await expect(page.locator('body')).toContainText(/mock key reveal proxy ok|200/);

    const requestListScreenshot = '/tmp/maxx-user-panel-key-request-list.png';
    await page.screenshot({ path: requestListScreenshot, fullPage: true });
    await testInfo.attach('user-panel-key-request-list', {
      path: requestListScreenshot,
      contentType: 'image/png',
    });
  } finally {
    await adminAPI(
      'PUT',
      '/settings/ui_multitenant_enabled',
      { value: previousSettings.ui_multitenant_enabled ?? 'false' },
      adminToken,
    ).catch(() => undefined);
    await adminAPI(
      'PUT',
      '/settings/ui_multitenant_layout',
      { value: previousSettings.ui_multitenant_layout ?? 'admin_panel' },
      adminToken,
    ).catch(() => undefined);
    if (routeId) {
      await adminAPI('DELETE', `/routes/${routeId}`, undefined, adminToken).catch(() => undefined);
    }
    if (providerId) {
      await adminAPI('DELETE', `/providers/${providerId}`, undefined, adminToken).catch(() => undefined);
    }
    if (createdUserId) {
      await adminAPI('DELETE', `/users/${createdUserId}`, undefined, adminToken).catch(() => undefined);
    }
    await closeServer(mock.server).catch(() => undefined);
  }
});
