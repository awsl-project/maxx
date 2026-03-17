/**
 * Playwright E2E Test: API Tokens page renders recent IP metadata
 *
 * 使用方式：
 *   npx playwright test -c playwright.config.ts api-tokens-recent-ip.spec.ts --project=e2e-chromium
 */
import http from 'node:http';

import { expect, test } from 'playwright/test';

import { BASE, PASS, USER, adminAPI, loginToAdminUI, closeServer } from './helpers';

test.describe.configure({ mode: 'serial' });

function startMockOpenAIServer(): Promise<{ server: http.Server; port: number }> {
  return new Promise((resolve) => {
    const server = http.createServer((req, res) => {
      if (req.method === 'POST' && req.url?.includes('/v1/chat/completions')) {
        let body = '';
        req.on('data', (chunk) => {
          body += chunk;
        });
        req.on('end', () => {
          let parsed: any = {};
          try {
            parsed = JSON.parse(body);
          } catch {
            // ignore malformed JSON in mock
          }

          res.writeHead(200, { 'Content-Type': 'application/json' });
          res.end(
            JSON.stringify({
              id: `chatcmpl_mock_${Date.now()}`,
              object: 'chat.completion',
              created: Math.floor(Date.now() / 1000),
              model: parsed.model || 'gpt-4o-mini',
              choices: [
                {
                  index: 0,
                  message: {
                    role: 'assistant',
                    content: 'Hello from mock OpenAI!',
                  },
                  finish_reason: 'stop',
                },
              ],
              usage: {
                prompt_tokens: 12,
                completion_tokens: 8,
                total_tokens: 20,
              },
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
      console.log(`✅ Mock OpenAI API server started on port ${address.port}`);
      resolve({ server, port: address.port });
    });
  });
}

async function sendOpenAIRequest(apiToken: string, forwardedFor: string) {
  const response = await fetch(`${BASE}/v1/chat/completions`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${apiToken}`,
      'X-Forwarded-For': forwardedFor,
    },
    body: JSON.stringify({
      model: 'gpt-4o-mini',
      messages: [{ role: 'user', content: 'Hello!' }],
      max_tokens: 64,
    }),
  });

  const text = await response.text();
  if (!response.ok) {
    throw new Error(`Proxy request failed (${response.status}): ${text}`);
  }
  return JSON.parse(text);
}

test('api tokens page shows recent IP column, populated IP metadata, and never placeholder', async ({ page }, testInfo) => {
  const mock = await startMockOpenAIServer();
  let jwt: string | null = null;
  let previousApiTokenAuthEnabled: string | undefined;
  let providerId: number | null = null;
  let routeId: number | null = null;
  const tokenIds: number[] = [];

  try {
    console.log('\n--- Setup: Admin Login ---');
    const loginResponse = await adminAPI('POST', '/auth/login', {
      username: USER,
      password: PASS,
    });
    jwt = loginResponse.token as string;
    expect(jwt).toBeTruthy();
    console.log('✅ Admin login success');

    console.log('\n--- Setup: Enable API Token Auth ---');
    const settings = await adminAPI('GET', '/settings', undefined, jwt);
    previousApiTokenAuthEnabled = settings.api_token_auth_enabled;
    await adminAPI('PUT', '/settings/api_token_auth_enabled', { value: 'true' }, jwt);
    console.log('✅ API token auth enabled');

    console.log('\n--- Setup: Create Provider + Route ---');
    const provider = await adminAPI(
      'POST',
      '/providers',
      {
        name: `Mock OpenAI Provider ${Date.now()}`,
        type: 'custom',
        config: {
          custom: {
            baseURL: `http://127.0.0.1:${mock.port}`,
            apiKey: 'mock-key',
          },
        },
        supportedClientTypes: ['openai'],
        supportModels: ['*'],
      },
      jwt,
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
      jwt,
    );
    routeId = route.id;
    console.log(`✅ Provider/route ready: provider=${providerId}, route=${routeId}`);

    console.log('\n--- Setup: Create API Tokens ---');
    const usedTokenResult = await adminAPI(
      'POST',
      '/api-tokens',
      { name: 'Recent IP Used Token', description: 'Token that will receive recent IP data' },
      jwt,
    );
    const unusedTokenResult = await adminAPI(
      'POST',
      '/api-tokens',
      { name: 'Recent IP Unused Token', description: 'Token that should keep the never placeholder' },
      jwt,
    );
    tokenIds.push(usedTokenResult.apiToken.id, unusedTokenResult.apiToken.id);
    const usedToken = usedTokenResult.token as string;
    expect(usedToken).toBeTruthy();
    console.log('✅ API tokens created');

    console.log('\n--- Setup: Send Proxy Request Through Used Token ---');
    const forwardedFor = '198.51.100.42';
    expect((await sendOpenAIRequest(usedToken, forwardedFor)).id).toBeTruthy();

    await expect
      .poll(async () => {
        const tokens = await adminAPI('GET', '/api-tokens', undefined, jwt ?? undefined);
        const target = tokens.find((entry: any) => entry.id === usedTokenResult.apiToken.id);
        return {
          lastIP: target?.lastIP ?? null,
          hasLastIPAt: Boolean(target?.lastIPAt),
          hasLastUsedAt: Boolean(target?.lastUsedAt),
        };
      }, { timeout: 15000 })
      .toEqual({
        lastIP: forwardedFor,
        hasLastIPAt: true,
        hasLastUsedAt: true,
      });
    console.log('✅ Proxy request updated lastIP / lastIPAt / lastUsedAt');

    console.log('\n--- Step 1: Browser Login ---');
    await loginToAdminUI(page);
    console.log('✅ Browser login success');

    console.log('\n--- Step 2: Navigate to API Tokens Page ---');
    await page.goto(`${BASE}/api-tokens`);
    await expect(page.locator('body')).toContainText(/API Tokens|API 令牌/, { timeout: 15000 });
    await expect(page.locator('th').filter({ hasText: /Recent IP|最近 IP/ })).toBeVisible();
    console.log('✅ API Tokens page loaded with Recent IP column');

    console.log('\n--- Step 3: Verify Used Token Row Shows IP Metadata ---');
    const usedRow = page.locator('tr').filter({ hasText: 'Recent IP Used Token' }).first();
    await expect(usedRow).toBeVisible({ timeout: 10000 });
    await expect(usedRow).toContainText('198.51.100.42');
    await expect(usedRow).not.toContainText(/Never|从未/);
    console.log('✅ Used token row shows recent IP + timestamps');

    console.log('\n--- Step 4: Verify Unused Token Row Keeps Never Placeholder ---');
    const unusedRow = page.locator('tr').filter({ hasText: 'Recent IP Unused Token' }).first();
    await expect(unusedRow).toBeVisible({ timeout: 10000 });
    await expect(unusedRow).toContainText(/Never|从未/);
    console.log('✅ Unused token row shows never placeholder');

    const screenshotPath = '/tmp/api-tokens-recent-ip-result.png';
    const screenshot = await page.screenshot({ path: screenshotPath, fullPage: true });
    await testInfo.attach('api-tokens-recent-ip-result', {
      body: screenshot,
      contentType: 'image/png',
    });
    console.log(`Screenshot: ${screenshotPath}`);
  } finally {
    for (const id of tokenIds.reverse()) {
      try {
        await adminAPI('DELETE', `/api-tokens/${id}`, undefined, jwt ?? undefined);
      } catch {}
    }
    if (routeId) {
      try {
        await adminAPI('DELETE', `/routes/${routeId}`, undefined, jwt ?? undefined);
      } catch {}
    }
    if (providerId) {
      try {
        await adminAPI('DELETE', `/providers/${providerId}`, undefined, jwt ?? undefined);
      } catch {}
    }
    if (previousApiTokenAuthEnabled !== undefined) {
      try {
        await adminAPI(
          'PUT',
          '/settings/api_token_auth_enabled',
          { value: previousApiTokenAuthEnabled },
          jwt ?? undefined,
        );
      } catch {}
    }
    await closeServer(mock.server);
  }
});
