/**
 * Playwright E2E Test: Requests Page - Loading State Under Delayed Filter Dependency
 *
 * Regression test for #465: under high concurrency, the requests page showed
 * "暂无请求记录" instead of a loading spinner when filter dependencies (e.g.
 * the providers list) were slow to resolve.
 *
 * 使用方式：
 *   npx playwright test -c playwright.config.ts requests-loading-state.spec.ts --project=e2e-chromium
 */
import http from 'node:http';

import { expect, test } from 'playwright/test';

import { BASE, adminAPI, loginToAdminAPI, loginToAdminUI } from './helpers';

/** Starts a minimal mock Claude API server that returns a canned response for /v1/messages. */
function startMockClaudeServer(): Promise<{ server: http.Server; port: number }> {
  return new Promise((resolve) => {
    const server = http.createServer((req, res) => {
      if (req.method !== 'POST' || !req.url?.startsWith('/v1/messages')) {
        res.writeHead(404, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ error: 'not found' }));
        return;
      }
      let body = '';
      req.on('data', (c) => {
        body += c;
      });
      req.on('end', () => {
        let parsed: any = {};
        try {
          parsed = JSON.parse(body);
        } catch {
          // Non-JSON body is fine for mock purposes; use default empty object
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
      const addr = server.address();
      if (!addr || typeof addr === 'string') throw new Error('no port');
      resolve({ server, port: addr.port });
    });
  });
}

/** Gracefully closes the mock server with a 2s timeout fallback. */
function closeMockServer(server: http.Server): Promise<void> {
  return new Promise((resolve) => {
    const timeout = setTimeout(() => {
      server.closeAllConnections?.();
      resolve();
    }, 2000);
    server.close(() => {
      clearTimeout(timeout);
      resolve();
    });
  });
}

test('requests page shows loading fallback when filter dependency is delayed', async ({
  page,
}) => {
  const mock = await startMockClaudeServer();
  let jwt: string | undefined;
  let providerId: number | null = null;
  let routeId: number | null = null;
  let previousApiTokenAuthEnabled: string | undefined;

  try {
    jwt = await loginToAdminAPI();

    // Disable token auth so proxy requests work without API tokens
    const settings = await adminAPI('GET', '/settings', undefined, jwt);
    previousApiTokenAuthEnabled = settings.api_token_auth_enabled;
    await adminAPI('PUT', '/settings/api_token_auth_enabled', { value: 'false' }, jwt);

    // Create provider pointing to mock server
    const provider = await adminAPI(
      'POST',
      '/providers',
      {
        name: `Loading Test ${Date.now()}`,
        type: 'custom',
        config: { custom: { baseURL: `http://127.0.0.1:${mock.port}`, apiKey: 'mock-key' } },
        supportedClientTypes: ['claude'],
        supportModels: ['*'],
      },
      jwt,
    );
    providerId = provider.id;

    // Create route for the provider
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

    // Send a proxy request to generate data
    const proxyRes = await fetch(`${BASE}/v1/messages`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'anthropic-version': '2023-06-01' },
      body: JSON.stringify({
        model: `loading-test-${Date.now()}`,
        max_tokens: 32,
        messages: [{ role: 'user', content: 'hi' }],
      }),
    });
    expect(proxyRes.ok).toBe(true);

    // Wait for the request to be recorded in the backend
    await expect
      .poll(
        async () => {
          const res = await adminAPI(
            'GET',
            `/requests?limit=5&providerId=${providerId}`,
            undefined,
            jwt,
          );
          return res.items?.length ?? 0;
        },
        { timeout: 10_000 },
      )
      .toBeGreaterThan(0);

    // Login first (unconditionally — avoids flaky SPA auth detection)
    await loginToAdminUI(page);

    // Set localStorage to use provider filter (simulates returning user)
    await page.evaluate(
      ({ pid }) => {
        localStorage.setItem('maxx-requests-filter-mode', 'provider');
        localStorage.setItem('maxx-requests-provider-filter', String(pid));
      },
      { pid: providerId },
    );

    // Delay the providers API by 1.5s to simulate slow filter dependency
    await page.route('**/api/admin/providers', async (route) => {
      await new Promise((r) => setTimeout(r, 1500));
      await route.continue();
    });

    // Navigate to requests page (already authenticated)
    await page.goto(`${BASE}/requests`);

    // KEY ASSERTION: during the providers API delay, must NOT show empty state
    // (should show a loading spinner instead). Playwright's negative web-first
    // assertion continuously monitors for the full timeout window.
    await expect(page.locator('body')).not.toContainText(
      /暂无请求记录|No requests recorded/,
      { timeout: 1500 },
    );

    // After the delay resolves, verify actual request rows are rendered
    // (not just header text which could match "0 total requests")
    await expect(page.locator('tr[data-request-row="true"]').first()).toBeVisible({
      timeout: 15_000,
    });
  } finally {
    if (jwt && previousApiTokenAuthEnabled !== undefined) {
      await adminAPI(
        'PUT',
        '/settings/api_token_auth_enabled',
        { value: previousApiTokenAuthEnabled },
        jwt,
      ).catch(() => {});
    }
    if (jwt && routeId) {
      await adminAPI('DELETE', `/routes/${routeId}`, undefined, jwt).catch(() => {});
    }
    if (jwt && providerId) {
      await adminAPI('DELETE', `/providers/${providerId}`, undefined, jwt).catch(() => {});
    }
    await closeMockServer(mock.server);
  }
});
