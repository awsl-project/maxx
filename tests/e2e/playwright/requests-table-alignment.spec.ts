import http from 'node:http';

import { expect, test, type Page } from 'playwright/test';

import {
  BASE,
  adminAPI,
  closeServer,
  loginToAdminAPI,
  loginToAdminUI,
} from './helpers';

test.describe.configure({ mode: 'serial' });

type TableGeometry = {
  label: string;
  headers: Array<{ text: string; x: number; width: number }>;
  cells: Array<{ text: string; x: number; width: number }>;
  rowCount: number;
  tbodyRows: number;
};

function startMockClaudeServer(): Promise<{ server: http.Server; port: number }> {
  return new Promise((resolve) => {
    const server = http.createServer((req, res) => {
      if (req.method === 'POST' && req.url?.includes('/v1/messages')) {
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

          res.writeHead(200, { 'Content-Type': 'application/json' });
          res.end(
            JSON.stringify({
              id: `msg_mock_${Date.now()}`,
              type: 'message',
              role: 'assistant',
              model: parsed.model || 'claude-sonnet-4-20250514',
              content: [{ type: 'text', text: 'Hello from mock Claude!' }],
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

    server.listen(0, '127.0.0.1', () => {
      const address = server.address();
      if (!address || typeof address === 'string') {
        throw new Error('Failed to determine mock server port');
      }
      resolve({ server, port: address.port });
    });
  });
}

async function sendClaudeRequest(model = 'claude-sonnet-4-20250514') {
  const response = await fetch(`${BASE}/v1/messages`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'anthropic-version': '2023-06-01',
    },
    body: JSON.stringify({
      model,
      max_tokens: 100,
      messages: [{ role: 'user', content: 'Hello!' }],
    }),
  });

  const text = await response.text();
  if (!response.ok) {
    throw new Error(`Proxy request failed (${response.status}): ${text}`);
  }
  return JSON.parse(text);
}

async function readTableGeometry(page: Page, label: string): Promise<TableGeometry> {
  return page.evaluate((sampleLabel) => {
    const headers = Array.from(document.querySelectorAll('table thead th')).map((th) => {
      const rect = th.getBoundingClientRect();
      return {
        text: (th.textContent || '').trim(),
        x: Math.round(rect.x),
        width: Math.round(rect.width),
      };
    });

    const firstRow = document.querySelector('tbody tr[data-request-row="true"]');
    const cells = firstRow
      ? Array.from(firstRow.querySelectorAll('td')).map((td) => {
          const rect = td.getBoundingClientRect();
          return {
            text: (td.textContent || '').trim().slice(0, 40),
            x: Math.round(rect.x),
            width: Math.round(rect.width),
          };
        })
      : [];

    return {
      label: sampleLabel,
      headers,
      cells,
      rowCount: document.querySelectorAll('tbody tr[data-request-row="true"]').length,
      tbodyRows: document.querySelectorAll('tbody tr').length,
    };
  }, label);
}

function expectColumnsAligned(sample: TableGeometry) {
  expect(sample.headers.length, `${sample.label} should render table headers`).toBeGreaterThan(0);
  expect(sample.cells.length, `${sample.label} should render at least one request row`).toBeGreaterThan(0);
  expect(sample.headers.length).toBe(sample.cells.length);

  sample.headers.forEach((header, index) => {
    const cell = sample.cells[index];
    expect(
      Math.abs(header.x - cell.x),
      `${sample.label} column ${index} (${header.text}) x mismatch: header=${header.x}, cell=${cell.x}`,
    ).toBeLessThanOrEqual(1);
    expect(
      Math.abs(header.width - cell.width),
      `${sample.label} column ${index} (${header.text}) width mismatch: header=${header.width}, cell=${cell.width}`,
    ).toBeLessThanOrEqual(1);
  });
}

async function scrollRequestsTable(page: Page, ratio: number) {
  await page.evaluate((targetRatio) => {
    const table = document.querySelector('table');
    const container = table?.parentElement;
    if (!(container instanceof HTMLElement)) {
      throw new Error('Failed to locate the requests scroll container');
    }

    const maxScrollTop = Math.max(0, container.scrollHeight - container.clientHeight);
    container.scrollTop = Math.round(maxScrollTop * targetRatio);
  }, ratio);
}

async function resolveAdminToken() {
  try {
    return await loginToAdminAPI();
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    if (message.includes('(401)')) {
      return undefined;
    }
    throw error;
  }
}

async function openRequestsPage(page: Page) {
  await page.goto(`${BASE}/requests`);
  await page.waitForLoadState('networkidle');

  if (await page.locator('input[type="password"]').count()) {
    await loginToAdminUI(page);
    await page.goto(`${BASE}/requests`);
    await page.waitForLoadState('networkidle');
  }
}

test('virtualized requests table keeps header and body columns aligned', async ({ page }, testInfo) => {
  const mock = await startMockClaudeServer();
  let jwt: string | undefined;
  let providerId: number | null = null;
  let routeId: number | null = null;
  let previousApiTokenAuthEnabled: string | undefined;

  try {
    jwt = await resolveAdminToken();
    const settings = await adminAPI('GET', '/settings', undefined, jwt);
    previousApiTokenAuthEnabled = settings.api_token_auth_enabled;
    await adminAPI('PUT', '/settings/api_token_auth_enabled', { value: 'false' }, jwt);

    const suffix = Date.now();
    const provider = await adminAPI(
      'POST',
      '/providers',
      {
        name: `Alignment Mock ${suffix}`,
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

    for (let batch = 0; batch < 6; batch += 1) {
      await Promise.all(
        Array.from({ length: 10 }, (_, index) =>
          sendClaudeRequest(`claude-sonnet-4-20250514-b${batch}-r${index}`),
        ),
      );
    }

    await expect
      .poll(
        async () => {
          const requests = await adminAPI('GET', '/requests?limit=100', undefined, jwt);
          return requests.items?.filter((item: any) => item.providerID === providerId).length ?? 0;
        },
        { timeout: 15000 },
      )
      .toBeGreaterThanOrEqual(40);

    await openRequestsPage(page);
    await expect(page.locator('table thead th').first()).toBeVisible({ timeout: 30_000 });
    await expect
      .poll(async () => page.locator('tbody tr[data-request-row="true"]').count(), { timeout: 30_000 })
      .toBeGreaterThan(0);

    const top = await readTableGeometry(page, 'top');
    expect(top.rowCount).toBeLessThan(60);
    expectColumnsAligned(top);

    await scrollRequestsTable(page, 0.65);
    await page.waitForTimeout(300);

    const afterScroll = await readTableGeometry(page, 'after-scroll');
    expectColumnsAligned(afterScroll);

    await testInfo.attach('requests-table-alignment.json', {
      body: Buffer.from(JSON.stringify({ top, afterScroll }, null, 2)),
      contentType: 'application/json',
    });
  } finally {
    if (previousApiTokenAuthEnabled !== undefined) {
      try {
        await adminAPI(
          'PUT',
          '/settings/api_token_auth_enabled',
          { value: previousApiTokenAuthEnabled },
          jwt,
        );
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
