import http from 'node:http';

import { expect, test, type Page } from 'playwright/test';

import { BASE, adminAPI, closeServer, loginToAdminAPI, loginToAdminUI } from './helpers';

test.describe.configure({ mode: 'serial' });
test.setTimeout(180_000);

const REQUEST_FILTER_MODE_STORAGE_KEY = 'maxx-requests-filter-mode';
const REQUEST_PROVIDER_FILTER_STORAGE_KEY = 'maxx-requests-provider-filter';
const REQUEST_TOKEN_FILTER_STORAGE_KEY = 'maxx-requests-token-filter';
const REQUEST_PROJECT_FILTER_STORAGE_KEY = 'maxx-requests-project-filter';
const REQUEST_FILTER_MODE_SCOPED_STORAGE_KEY = 'maxx-requests-filter-mode:tenant-1:user-1';
const REQUEST_PROVIDER_FILTER_SCOPED_STORAGE_KEY = 'maxx-requests-provider-filter:tenant-1:user-1';
const REQUEST_TOKEN_FILTER_SCOPED_STORAGE_KEY = 'maxx-requests-token-filter:tenant-1:user-1';
const REQUEST_PROJECT_FILTER_SCOPED_STORAGE_KEY = 'maxx-requests-project-filter:tenant-1:user-1';

type MockHit = {
  id: number;
  method: string;
  url: string;
  model: string;
  status: number;
  scenario: string;
  at: string;
};

function htmlEscape(value: string): string {
  return value.replace(/[&<>'"]/g, (char) => {
    switch (char) {
      case '&':
        return '&amp;';
      case '<':
        return '&lt;';
      case '>':
        return '&gt;';
      case "'":
        return '&#39;';
      case '"':
        return '&quot;';
      default:
        return char;
    }
  });
}

function startTraceableMockClaudeServer(): Promise<{
  server: http.Server;
  port: number;
  inspectorUrl: string;
  hits: MockHit[];
}> {
  const hits: MockHit[] = [];

  return new Promise((resolve) => {
    const server = http.createServer((req, res) => {
      if (req.method === 'GET' && req.url === '/mock-inspector') {
        res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
        res.end(`<!doctype html>
<html>
  <head>
    <title>Maxx provider mock inspector</title>
    <style>
      body { font-family: Inter, system-ui, sans-serif; margin: 24px; color: #0f172a; }
      table { border-collapse: collapse; width: 100%; }
      th, td { border: 1px solid #cbd5e1; padding: 8px; text-align: left; font-size: 13px; }
      th { background: #f1f5f9; }
      .ok { color: #047857; font-weight: 700; }
      .error { color: #b91c1c; font-weight: 700; }
    </style>
  </head>
  <body>
    <h1>Provider mock interactions</h1>
    <p>Total hits: ${hits.length}</p>
    <table>
      <thead><tr><th>#</th><th>Method</th><th>URL</th><th>Model</th><th>Scenario</th><th>Status</th><th>At</th></tr></thead>
      <tbody>
        ${hits
          .map(
            (hit) => `<tr>
              <td>${hit.id}</td>
              <td>${htmlEscape(hit.method)}</td>
              <td>${htmlEscape(hit.url)}</td>
              <td>${htmlEscape(hit.model)}</td>
              <td>${htmlEscape(hit.scenario)}</td>
              <td class="${hit.status >= 500 ? 'error' : 'ok'}">${hit.status}</td>
              <td>${htmlEscape(hit.at)}</td>
            </tr>`,
          )
          .join('')}
      </tbody>
    </table>
  </body>
</html>`);
        return;
      }

      if (req.method !== 'POST' || !req.url?.includes('/v1/messages')) {
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
        } catch {
          // Keep malformed payloads visible in the inspector through the empty model field.
        }

        const model = String(parsed.model || 'claude-sonnet-4-20250514');
        const scenario = model.includes('layout-fail') ? 'provider-500' : 'success';
        const status = scenario === 'provider-500' ? 500 : 200;
        hits.push({
          id: hits.length + 1,
          method: req.method || 'POST',
          url: req.url || '/v1/messages',
          model,
          status,
          scenario,
          at: new Date().toISOString(),
        });

        if (scenario === 'provider-500') {
          res.writeHead(500, { 'Content-Type': 'application/json' });
          res.end(JSON.stringify({ error: { type: 'mock_error', message: 'layout regression mock failure' } }));
          return;
        }

        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(
          JSON.stringify({
            id: `msg_layout_${Date.now()}`,
            type: 'message',
            role: 'assistant',
            model,
            content: [{ type: 'text', text: 'layout e2e success' }],
            stop_reason: 'end_turn',
            stop_sequence: null,
            usage: {
              input_tokens: 21,
              output_tokens: 13,
              cache_creation_input_tokens: 0,
              cache_read_input_tokens: 0,
            },
          }),
        );
      });
    });

    server.listen(0, '127.0.0.1', () => {
      const address = server.address();
      if (!address || typeof address === 'string') {
        throw new Error('Failed to determine mock server port');
      }
      resolve({
        server,
        port: address.port,
        inspectorUrl: `http://127.0.0.1:${address.port}/mock-inspector`,
        hits,
      });
    });
  });
}

async function sendClaudeRequest(model: string) {
  const response = await fetch(`${BASE}/v1/messages`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'anthropic-version': '2023-06-01',
    },
    body: JSON.stringify({
      model,
      max_tokens: 100,
      messages: [{ role: 'user', content: `Exercise ${model}` }],
    }),
  });

  return { status: response.status, body: await response.text() };
}

async function openRequestsPageForProvider(page: Page, providerId: number) {
  await page.addInitScript(
    ({ id, keys }) => {
      localStorage.setItem(keys.mode, 'provider');
      localStorage.setItem(keys.provider, String(id));
      localStorage.removeItem(keys.token);
      localStorage.removeItem(keys.project);
      localStorage.setItem(keys.scopedMode, 'provider');
      localStorage.setItem(keys.scopedProvider, String(id));
      localStorage.removeItem(keys.scopedToken);
      localStorage.removeItem(keys.scopedProject);
    },
    {
      id: providerId,
      keys: {
        mode: REQUEST_FILTER_MODE_STORAGE_KEY,
        provider: REQUEST_PROVIDER_FILTER_STORAGE_KEY,
        token: REQUEST_TOKEN_FILTER_STORAGE_KEY,
        project: REQUEST_PROJECT_FILTER_STORAGE_KEY,
        scopedMode: REQUEST_FILTER_MODE_SCOPED_STORAGE_KEY,
        scopedProvider: REQUEST_PROVIDER_FILTER_SCOPED_STORAGE_KEY,
        scopedToken: REQUEST_TOKEN_FILTER_SCOPED_STORAGE_KEY,
        scopedProject: REQUEST_PROJECT_FILTER_SCOPED_STORAGE_KEY,
      },
    },
  );

  await page.goto(`${BASE}/requests`, { waitUntil: 'domcontentloaded' });
  if (await page.locator('input[type="password"]').isVisible().catch(() => false)) {
    await loginToAdminUI(page);
    await page.goto(`${BASE}/requests`, { waitUntil: 'domcontentloaded' });
  }

  await expect(page.getByRole('heading', { name: 'Requests' })).toBeVisible({ timeout: 30_000 });
}

test('requests tab layout exposes operations metrics, filters, details, and provider mock evidence', async ({
  page,
  context,
}, testInfo) => {
  const mock = await startTraceableMockClaudeServer();
  let jwt: string | undefined;
  let providerId: number | null = null;
  let routeId: number | null = null;
  let previousApiTokenAuthEnabled: string | undefined;

  try {
    jwt = await loginToAdminAPI();
    const settings = await adminAPI('GET', '/settings', undefined, jwt);
    previousApiTokenAuthEnabled = settings.api_token_auth_enabled;
    await adminAPI('PUT', '/settings/api_token_auth_enabled', { value: 'false' }, jwt);

    const suffix = Date.now();
    const provider = await adminAPI(
      'POST',
      '/providers',
      {
        name: `Layout Mock ${suffix}`,
        type: 'custom',
        config: {
          custom: {
            baseURL: `http://127.0.0.1:${mock.port}`,
            apiKey: 'mock-key-redacted',
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

    const success = await sendClaudeRequest(`claude-sonnet-4-20250514-layout-success-${suffix}`);
    expect(success.status).toBe(200);
    const failed = await sendClaudeRequest(`claude-sonnet-4-20250514-layout-fail-${suffix}`);
    expect(failed.status).toBeGreaterThanOrEqual(500);

    await expect
      .poll(
        async () => {
          const requests = await adminAPI('GET', `/requests?limit=20&providerId=${provider.id}`, undefined, jwt);
          const statuses = (requests.items ?? []).map((item: any) => item.status);
          return {
            count: requests.items?.length ?? 0,
            hasCompleted: statuses.includes('COMPLETED'),
            hasFailed: statuses.includes('FAILED'),
          };
        },
        { timeout: 30_000 },
      )
      .toEqual(expect.objectContaining({ count: expect.any(Number), hasCompleted: true, hasFailed: true }));

    await openRequestsPageForProvider(page, provider.id);
    await expect(page.getByTestId('requests-operations-panel')).toBeVisible({ timeout: 30_000 });
    await expect(page.getByTestId('requests-metric-total')).toContainText(/Total requests/i);
    await expect(page.getByTestId('requests-metric-error-rate')).toContainText(/Error rate/i);
    await expect(page.getByTestId('requests-metric-p95')).toContainText(/P95 latency/i);
    await expect
      .poll(async () => page.locator('tbody tr[data-request-row="true"]').count(), { timeout: 30_000 })
      .toBeGreaterThanOrEqual(2);

    const listScreenshotPath = testInfo.outputPath('requests-layout-list.png');
    await page.screenshot({ path: listScreenshotPath, fullPage: true });
    await testInfo.attach('requests-layout-list.png', {
      path: listScreenshotPath,
      contentType: 'image/png',
    });

    await page.getByRole('button', { name: /Errors only|只看错误/i }).click();
    await expect(page.locator('tbody tr[data-request-row="true"]').first()).toContainText(/Failed/i, {
      timeout: 30_000,
    });
    const errorFilterScreenshotPath = testInfo.outputPath('requests-layout-error-filter.png');
    await page.screenshot({ path: errorFilterScreenshotPath, fullPage: true });
    await testInfo.attach('requests-layout-error-filter.png', {
      path: errorFilterScreenshotPath,
      contentType: 'image/png',
    });

    await page.locator('tbody tr[data-request-row="true"]').first().click();
    await expect(page.getByText(/Request #/)).toBeVisible({ timeout: 30_000 });
    await expect(page.getByText(/layout regression mock failure/i)).toBeVisible({ timeout: 30_000 });
    const detailScreenshotPath = testInfo.outputPath('requests-layout-detail.png');
    await page.screenshot({ path: detailScreenshotPath, fullPage: true });
    await testInfo.attach('requests-layout-detail.png', {
      path: detailScreenshotPath,
      contentType: 'image/png',
    });

    const mockInspector = await context.newPage();
    await mockInspector.goto(mock.inspectorUrl, { waitUntil: 'domcontentloaded' });
    await expect(mockInspector.getByRole('heading', { name: 'Provider mock interactions' })).toBeVisible();
    await expect(mockInspector.getByText(/layout-success/)).toBeVisible();
    await expect(mockInspector.getByText(/layout-fail/)).toBeVisible();
    const mockScreenshotPath = testInfo.outputPath('requests-layout-provider-mock.png');
    await mockInspector.screenshot({ path: mockScreenshotPath, fullPage: true });
    await testInfo.attach('requests-layout-provider-mock.png', {
      path: mockScreenshotPath,
      contentType: 'image/png',
    });
    await mockInspector.close();
  } finally {
    if (previousApiTokenAuthEnabled !== undefined) {
      try {
        await adminAPI('PUT', '/settings/api_token_auth_enabled', { value: previousApiTokenAuthEnabled }, jwt);
      } catch {}
    }
    if (routeId) {
      try {
        await adminAPI('DELETE', `/routes/${routeId}`, undefined, jwt);
      } catch {}
    }
    if (providerId) {
      try {
        await adminAPI('DELETE', `/providers/${providerId}`, undefined, jwt);
      } catch {}
    }
    await closeServer(mock.server);
  }
});
