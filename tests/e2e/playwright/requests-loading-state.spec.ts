import http from 'node:http';

import { expect, test } from 'playwright/test';

import { BASE, adminAPI, loginToAdminAPI, loginToAdminUI } from './helpers';

test('requests page shows loading fallback first and then renders records under delayed filter dependency', async ({ page }) => {
  let jwt: string | undefined;
  const mock = await new Promise<{ server: http.Server; port: number }>((resolve) => {
    const server = http.createServer((req, res) => {
      if (req.method !== 'POST' || !req.url?.startsWith('/v1/messages')) {
        res.writeHead(404, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ error: 'not found' }));
        return;
      }
      let body = '';
      req.on('data', (chunk) => {
        body += chunk;
      });
      req.on('end', () => {
        let parsed: any = {};
        try {
          parsed = JSON.parse(body);
        } catch (error) {
          console.warn('Mock server: failed to parse request body', { error, body });
        }
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(
          JSON.stringify({
            id: `msg_${Date.now()}`,
            type: 'message',
            role: 'assistant',
            model: parsed.model || 'claude-sonnet-4-20250514',
            content: [{ type: 'text', text: 'ok' }],
            stop_reason: 'end_turn',
            usage: { input_tokens: 10, output_tokens: 5 },
          }),
        );
      });
    });
    server.listen(0, '127.0.0.1', () => {
      const address = server.address();
      if (!address || typeof address === 'string') throw new Error('no port');
      resolve({ server, port: address.port });
    });
  });

  let providerId: number | null = null;
  let routeId: number | null = null;
  let previousApiTokenAuthEnabled: string | undefined;
  try {
    jwt = await loginToAdminAPI();
    const settings = await adminAPI('GET', '/settings', undefined, jwt);
    previousApiTokenAuthEnabled = settings.api_token_auth_enabled;
    await adminAPI('PUT', '/settings/api_token_auth_enabled', { value: 'false' }, jwt);
    const provider = await adminAPI(
      'POST',
      '/providers',
      {
        name: `Loading State Provider ${Date.now()}`,
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
        position: 1,
      },
      jwt,
    );
    routeId = route.id;

    const proxyResponse = await fetch(`${BASE}/v1/messages`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'anthropic-version': '2023-06-01',
      },
      body: JSON.stringify({
        model: `loading-state-${Date.now()}`,
        max_tokens: 32,
        messages: [{ role: 'user', content: 'hi' }],
      }),
    });
    if (!proxyResponse.ok) {
      throw new Error(`proxy request failed (${proxyResponse.status}): ${await proxyResponse.text()}`);
    }

    await page.addInitScript(
      ({ providerId }) => {
        localStorage.setItem('maxx-requests-filter-mode', 'provider');
        localStorage.setItem('maxx-requests-provider-filter', String(providerId));
      },
      { providerId },
    );

    await page.route('**/api/admin/providers', async (route) => {
      await new Promise((resolve) => setTimeout(resolve, 1500));
      await route.continue();
    });

    await page.goto(`${BASE}/requests`);
    await page.waitForLoadState('networkidle');
    if (await page.locator('input[type="password"]').count()) {
      await loginToAdminUI(page);
      await page.goto(`${BASE}/requests`);
      await page.waitForLoadState('networkidle');
    }

    await expect(page.locator('body')).not.toContainText(/暂无请求记录|No requests recorded/, {
      timeout: 1000,
    });

    await expect
      .poll(
        async () => {
          const response = await adminAPI('GET', `/requests?limit=20&providerId=${providerId}`, undefined, jwt);
          return response.items?.length ?? 0;
        },
        { timeout: 15000 },
      )
      .toBeGreaterThan(0);

    await expect(page.locator('svg.animate-spin').first()).toBeHidden({ timeout: 10000 });
    await expect(page.locator('body')).not.toContainText(/暂无请求记录|No requests recorded/, {
      timeout: 10000,
    });
    await expect(page.locator('body')).toContainText(/total requests|个请求/i, { timeout: 10000 });
  } finally {
    if (jwt && previousApiTokenAuthEnabled !== undefined) {
      await adminAPI(
        'PUT',
        '/settings/api_token_auth_enabled',
        { value: previousApiTokenAuthEnabled },
        jwt,
      ).catch(() => undefined);
    }
    if (jwt && routeId) {
      await adminAPI('DELETE', `/routes/${routeId}`, undefined, jwt).catch(() => undefined);
    }
    if (jwt && providerId) {
      await adminAPI('DELETE', `/providers/${providerId}`, undefined, jwt).catch(() => undefined);
    }
    await new Promise<void>((resolve) => {
      mock.server.close(() => resolve());
    });
  }
});
