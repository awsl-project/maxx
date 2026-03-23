/**
 * Playwright E2E Test: Provider-scoped proxy route
 *
 * 使用方式：
 *   npx playwright test -c playwright.config.ts provider-proxy-route.spec.ts --project=e2e-chromium
 */
import http from 'node:http';

import { expect, test } from 'playwright/test';

import { BASE, PASS, USER, adminAPI } from './helpers';

test.describe.configure({ mode: 'serial' });

function startMockClaudeServer(): Promise<{ server: http.Server; port: number }> {
  return new Promise((resolve, reject) => {
    const server = http.createServer((req, res) => {
      if (req.method === 'POST' && req.url?.startsWith('/v1/messages')) {
        let body = '';
        req.on('data', (chunk) => {
          body += chunk;
        });
        req.on('end', () => {
          let parsed: any = {};
          try {
            parsed = JSON.parse(body);
          } catch {
            // ignore malformed JSON in the mock
          }

          const model = parsed.model || 'claude-sonnet-4-20250514';
          res.writeHead(200, { 'Content-Type': 'application/json' });
          res.end(
            JSON.stringify({
              id: `msg_provider_proxy_${Date.now()}`,
              type: 'message',
              role: 'assistant',
              model,
              content: [{ type: 'text', text: 'Hello from provider-scoped mock Claude!' }],
              stop_reason: 'end_turn',
              stop_sequence: null,
              usage: {
                input_tokens: 15,
                output_tokens: 8,
                cache_creation_input_tokens: 0,
                cache_read_input_tokens: 0,
              },
            }),
          );
        });
        return;
      }

      res.writeHead(404, { 'Content-Type': 'application/json' });
      res.end(JSON.stringify({ error: 'not found' }));
    });

    server.once('error', reject);
    server.listen(0, '127.0.0.1', () => {
      const address = server.address();
      if (!address || typeof address === 'string') {
        reject(new Error('Failed to determine mock server port'));
        return;
      }
      resolve({ server, port: address.port });
    });
  });
}

test('provider-scoped proxy route records requests and shows provider URL in the UI', async ({ page }, testInfo) => {
  const mock = await startMockClaudeServer();
  let jwt: string | null = null;
  let providerId: number | null = null;
  let routeId: number | null = null;
  let tokenId: number | null = null;
  let apiToken: string | null = null;
  let previousApiTokenAuthEnabled: string | undefined;

  try {
    const loginResponse = await adminAPI('POST', '/auth/login', {
      username: USER,
      password: PASS,
    });
    jwt = loginResponse.token as string;
    expect(jwt).toBeTruthy();

    const settings = await adminAPI('GET', '/settings', undefined, jwt);
    previousApiTokenAuthEnabled = settings.api_token_auth_enabled;
    await adminAPI('PUT', '/settings/api_token_auth_enabled', { value: 'true' }, jwt);

    const provider = await adminAPI(
      'POST',
      '/providers',
      {
        name: 'Provider Scoped Claude UI',
        type: 'custom',
        config: {
          custom: {
            baseURL: `http://127.0.0.1:${mock.port}`,
            apiKey: 'mock-key',
          },
        },
        supportedClientTypes: ['claude'],
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
        clientType: 'claude',
        providerID: provider.id,
        projectID: 0,
        position: 1,
      },
      jwt,
    );
    routeId = route.id;

    const tokenResult = await adminAPI(
      'POST',
      '/api-tokens',
      { name: 'Provider Scoped Route Token', description: 'Playwright token for provider-scoped route' },
      jwt,
    );
    tokenId = tokenResult.apiToken.id;
    apiToken = tokenResult.token as string;
    expect(apiToken).toBeTruthy();

    const requestModel = `claude-sonnet-4-20250514-provider-ui-${Date.now()}`;
    const expectedProviderProxyUrl = `${new URL(BASE).origin}/provider/${provider.id}/`;
    const response = await fetch(`${BASE}/provider/${provider.id}/v1/messages`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'anthropic-version': '2023-06-01',
        'x-api-key': apiToken!,
      },
      body: JSON.stringify({
        model: requestModel,
        max_tokens: 100,
        messages: [{ role: 'user', content: 'Hello provider scoped route!' }],
      }),
    });
    const text = await response.text();
    if (!response.ok) {
      throw new Error(`Provider-scoped proxy request failed (${response.status}): ${text}`);
    }
    const payload = JSON.parse(text);
    expect(payload.model).toBe(requestModel);

    await page.goto(BASE);
    await expect(page.locator('input[type="text"]')).toBeVisible({ timeout: 10000 });
    await page.fill('input[type="text"]', USER);
    await page.fill('input[type="password"]', PASS);
    await page.locator('button[type="submit"]').click();
    await expect(page.locator('body')).toContainText(/Dashboard|dashboard/, { timeout: 10000 });

    await page.goto(`${BASE}/requests`);
    await expect(page.locator('body')).toContainText(/Requests|请求/, { timeout: 15000 });

    await expect
      .poll(async () => {
        const content = (await page.textContent('body')) ?? '';
        return content.includes(requestModel);
      }, { timeout: 15000 })
      .toBe(true);

    await page.goto(`${BASE}/providers/${provider.id}/edit`);
    await expect(page.locator('[data-testid="provider-proxy-url"]')).toBeVisible({ timeout: 10000 });
    await expect(page.locator('[data-testid="provider-proxy-url"]')).toContainText(
      expectedProviderProxyUrl,
      { timeout: 10000 },
    );

    await page.screenshot({ path: testInfo.outputPath('provider-edit-url.png'), fullPage: true });
  } finally {
    if (tokenId) {
      await adminAPI('DELETE', `/api-tokens/${tokenId}`, undefined, jwt ?? undefined).catch(() => undefined);
    }
    if (routeId) {
      await adminAPI('DELETE', `/routes/${routeId}`, undefined, jwt ?? undefined).catch(() => undefined);
    }
    if (providerId) {
      await adminAPI('DELETE', `/providers/${providerId}`, undefined, jwt ?? undefined).catch(() => undefined);
    }
    if (previousApiTokenAuthEnabled !== undefined) {
      await adminAPI(
        'PUT',
        '/settings/api_token_auth_enabled',
        { value: previousApiTokenAuthEnabled },
        jwt ?? undefined,
      ).catch(() => undefined);
    }
    await new Promise<void>((resolve, reject) => {
      mock.server.close((error?: Error) => {
        if (error) {
          reject(error);
          return;
        }
        resolve();
      });
    });
  }
});
