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
test.setTimeout(120_000);

const REQUEST_FILTER_MODE_STORAGE_KEY = 'maxx-requests-filter-mode';
const REQUEST_PROVIDER_FILTER_STORAGE_KEY = 'maxx-requests-provider-filter';
const REQUEST_TOKEN_FILTER_STORAGE_KEY = 'maxx-requests-token-filter';
const REQUEST_PROJECT_FILTER_STORAGE_KEY = 'maxx-requests-project-filter';

const ACTIVE_STATUS_RE = /pending|stream|waiting|in progress|等待|传输|绑定/i;
const TERMINAL_STATUS_RE = /completed|failed|cancelled|rejected|完成|失败|取消|拒绝/i;

/**
 * Waits for a fixed delay inside the reconnect test mock server.
 */
function delay(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

/**
 * Starts a mock Claude-compatible provider whose responses complete while the
 * requests page WebSocket is intentionally disconnected.
 */
function startReconnectMockServer(): Promise<{ server: http.Server; port: number }> {
  return new Promise((resolve) => {
    const server = http.createServer((req, res) => {
      if (req.method !== 'POST' || !req.url?.includes('/v1/messages')) {
        res.writeHead(404, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ error: 'not found' }));
        return;
      }

      let body = '';
      req.on('data', (chunk) => {
        body += chunk;
      });

      req.on('end', async () => {
        let parsed: any = {};
        try {
          parsed = JSON.parse(body);
        } catch {
          // ignore malformed JSON in mock
        }

        const model = String(parsed.model || '');
        const isSlowSuccess = model.includes('__slow-success');
        await delay(isSlowSuccess ? 3_000 : 80);

        if (res.writableEnded) {
          return;
        }

        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(
          JSON.stringify({
            id: `msg_mock_${Date.now()}_${Math.random().toString(36).slice(2, 8)}`,
            type: 'message',
            role: 'assistant',
            model,
            content: [{ type: 'text', text: `model=${model}` }],
            stop_reason: 'end_turn',
            stop_sequence: null,
            usage: {
              input_tokens: 12,
              output_tokens: 18,
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
        throw new Error('Failed to determine reconnect mock server port');
      }
      resolve({ server, port: address.port });
    });
  });
}

/**
 * Reuses the current admin session when possible and falls back to API login
 * when admin auth is enabled.
 */
async function resolveAdminToken() {
  try {
    await adminAPI('GET', '/settings');
    return undefined;
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    if (message.includes('(401)') || message.includes('(403)')) {
      return await loginToAdminAPI();
    }
    throw error;
  }
}

/**
 * Opens the requests page with a provider filter preloaded into localStorage.
 */
async function openRequestsPage(page: Page, providerId: number) {
  await page.addInitScript(
    ({ id, keys }) => {
      localStorage.setItem(keys.mode, 'provider');
      localStorage.setItem(keys.provider, String(id));
      localStorage.removeItem(keys.token);
      localStorage.removeItem(keys.project);
    },
    {
      id: providerId,
      keys: {
        mode: REQUEST_FILTER_MODE_STORAGE_KEY,
        provider: REQUEST_PROVIDER_FILTER_STORAGE_KEY,
        token: REQUEST_TOKEN_FILTER_STORAGE_KEY,
        project: REQUEST_PROJECT_FILTER_STORAGE_KEY,
      },
    },
  );

  await page.goto(`${BASE}/requests`);
  await page.waitForLoadState('networkidle');

  if (await page.locator('input[type="password"]').count()) {
    await loginToAdminUI(page);
    await page.goto(`${BASE}/requests`);
    await page.waitForLoadState('networkidle');
  }
}

/**
 * Installs a browser-side WebSocket shim that can force disconnects and block
 * reconnect attempts without taking HTTP traffic offline.
 */
async function installWebSocketTestHarness(page: Page) {
  await page.addInitScript(() => {
    const NativeWebSocket = window.WebSocket;
    const trackedSockets = new Set<WebSocket>();
    let blocked = false;

    const trackSocket = (ws: WebSocket) => {
      trackedSockets.add(ws);
      ws.addEventListener('close', () => {
        trackedSockets.delete(ws);
      });
      return ws;
    };

    class BlockedWebSocket implements Partial<WebSocket> {
      url: string;
      readyState = NativeWebSocket.CLOSED;
      bufferedAmount = 0;
      extensions = '';
      protocol = '';
      binaryType: BinaryType = 'blob';
      onopen: ((this: WebSocket, ev: Event) => any) | null = null;
      onerror: ((this: WebSocket, ev: Event) => any) | null = null;
      onclose: ((this: WebSocket, ev: CloseEvent) => any) | null = null;
      onmessage: ((this: WebSocket, ev: MessageEvent<any>) => any) | null = null;

      constructor(url: string) {
        this.url = url;
        queueMicrotask(() => {
          this.onerror?.call(this as WebSocket, new Event('error'));
          this.onclose?.call(
            this as WebSocket,
            new CloseEvent('close', { code: 1006, reason: 'blocked by playwright test' }),
          );
        });
      }

      addEventListener() {}

      removeEventListener() {}

      dispatchEvent() {
        return true;
      }

      close() {}

      send() {
        throw new DOMException('WebSocket is blocked by playwright test', 'InvalidStateError');
      }
    }

    const PatchedWebSocket = function (
      url: string | URL,
      protocols?: string | string[],
    ): WebSocket {
      if (blocked) {
        return new BlockedWebSocket(String(url)) as WebSocket;
      }

      const ws =
        protocols !== undefined ? new NativeWebSocket(url, protocols) : new NativeWebSocket(url);
      return trackSocket(ws);
    } as unknown as typeof WebSocket;

    PatchedWebSocket.prototype = NativeWebSocket.prototype;
    Object.setPrototypeOf(PatchedWebSocket, NativeWebSocket);
    Object.defineProperties(PatchedWebSocket, {
      CONNECTING: { value: NativeWebSocket.CONNECTING },
      OPEN: { value: NativeWebSocket.OPEN },
      CLOSING: { value: NativeWebSocket.CLOSING },
      CLOSED: { value: NativeWebSocket.CLOSED },
    });

    window.WebSocket = PatchedWebSocket;
    Object.defineProperty(window, '__maxxWsTest', {
      value: {
        setBlocked(next: boolean) {
          blocked = next;
          if (!next) {
            return;
          }
          for (const ws of Array.from(trackedSockets)) {
            try {
              ws.close();
            } catch {
              // ignore close failures in test harness
            }
          }
        },
      },
      configurable: false,
    });
  });
}

/**
 * Sends one project-routed Claude request with an explicit session id so the
 * reconnect regression does not inherit session bindings from earlier specs.
 */
async function sendProjectRequest(url: string, model: string, sessionId: string) {
  const response = await fetch(url, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'anthropic-version': '2023-06-01',
      'X-Session-Id': sessionId,
    },
    body: JSON.stringify({
      model,
      max_tokens: 64,
      messages: [{ role: 'user', content: `issue-413-${model}` }],
    }),
  });

  const text = await response.text();
  if (!response.ok) {
    throw new Error(`Project request failed (${response.status}): ${text}`);
  }
  return text;
}

/**
 * Locates the rendered requests table row for an exact request model.
 */
async function getRowByModel(page: Page, model: string) {
  return page.locator('tbody tr[data-request-row="true"]', {
    has: page.getByText(model, { exact: true }),
  });
}

test('requests page resyncs list and count after ws reconnect', async ({ page }) => {
  const mock = await startReconnectMockServer();
  let jwt: string | undefined;
  let previousApiTokenAuthEnabled: string | undefined;
  let providerId: number | null = null;
  let projectId: number | null = null;
  let routeId: number | null = null;

  try {
    jwt = await resolveAdminToken();
    const settings = await adminAPI('GET', '/settings', undefined, jwt);
    previousApiTokenAuthEnabled = settings.api_token_auth_enabled;
    await adminAPI('PUT', '/settings/api_token_auth_enabled', { value: 'false' }, jwt);

    const ts = Date.now();
    const provider = await adminAPI(
      'POST',
      '/providers',
      {
        name: `Issue 413 Provider ${ts}`,
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

    const project = await adminAPI(
      'POST',
      '/projects',
      {
        name: `Issue 413 Project ${ts}`,
        slug: `issue-413-${ts}`,
        enabledCustomRoutes: ['claude'],
      },
      jwt,
    );
    projectId = project.id;

    const route = await adminAPI(
      'POST',
      '/routes',
      {
        isEnabled: true,
        isNative: false,
        clientType: 'claude',
        providerID: provider.id,
        projectID: project.id,
        position: 1,
      },
      jwt,
    );
    routeId = route.id;

    const projectURL = `${BASE}/project/${project.slug}/v1/messages`;
    const slowModel = `claude-sonnet-4-20250514__slow-success__issue-413-a-${ts}`;
    const fastModel = `claude-sonnet-4-20250514__fast-success__issue-413-b-${ts}`;
    const sessionId = `issue-413-session-${ts}`;

    await installWebSocketTestHarness(page);
    await openRequestsPage(page, provider.id);
    await expect(page.locator('body')).toContainText(/Requests|请求/, { timeout: 30_000 });

    const firstRequestPromise = sendProjectRequest(projectURL, slowModel, sessionId);

    await expect
      .poll(
        async () => {
          const response = await adminAPI(
            'GET',
            `/requests?limit=20&providerId=${provider.id}`,
            undefined,
            jwt,
          );
          return response.items?.some((item: any) => String(item.requestModel) === slowModel) ?? false;
        },
        { timeout: 20_000 },
      )
      .toBe(true);

    const slowRow = await getRowByModel(page, slowModel);
    await expect(slowRow).toContainText(ACTIVE_STATUS_RE, { timeout: 20_000 });

    await page.evaluate(() => {
      (window as { __maxxWsTest?: { setBlocked(next: boolean): void } }).__maxxWsTest?.setBlocked(
        true,
      );
    });

    await firstRequestPromise;
    await sendProjectRequest(projectURL, fastModel, sessionId);

    await expect
      .poll(
        async () => {
          const response = await adminAPI(
            'GET',
            `/requests?limit=20&providerId=${provider.id}`,
            undefined,
            jwt,
          );
          const models = (response.items ?? []).map((item: any) => String(item.requestModel));
          const statuses = new Map(
            (response.items ?? []).map((item: any) => [String(item.requestModel), String(item.status)]),
          );
          return {
            count: response.items?.length ?? 0,
            hasSlow: models.includes(slowModel),
            hasFast: models.includes(fastModel),
            slowStatus: statuses.get(slowModel),
            fastStatus: statuses.get(fastModel),
          };
        },
        { timeout: 20_000 },
      )
      .toEqual({
        count: 2,
        hasSlow: true,
        hasFast: true,
        slowStatus: 'COMPLETED',
        fastStatus: 'COMPLETED',
      });

    const fastRowWhileDisconnected = await getRowByModel(page, fastModel);
    await expect(slowRow).toContainText(ACTIVE_STATUS_RE, { timeout: 5_000 });
    await expect(fastRowWhileDisconnected).toHaveCount(0);

    await page.evaluate(() => {
      (window as { __maxxWsTest?: { setBlocked(next: boolean): void } }).__maxxWsTest?.setBlocked(
        false,
      );
    });

    await expect
      .poll(
        async () => {
          const slow = await getRowByModel(page, slowModel);
          const fast = await getRowByModel(page, fastModel);
          return {
            slowVisible: await slow.count(),
            fastVisible: await fast.count(),
            slowText: await slow.textContent(),
            fastText: await fast.textContent(),
          };
        },
        { timeout: 30_000, intervals: [500, 1_000, 2_000, 4_000] },
      )
      .toMatchObject({
        slowVisible: 1,
        fastVisible: 1,
      });

    await expect
      .poll(
        async () => {
          const summaryText = (await page.locator('header p.text-xs').first().textContent()) ?? '';
          const normalized = summaryText.replace(/\s+/g, ' ').trim().toLowerCase();
          return /\b2\s*(total\s*)?requests\b/.test(normalized) || /共\s*2\s*个请求/.test(summaryText);
        },
        { timeout: 30_000, intervals: [500, 1_000, 2_000, 4_000] },
      )
      .toBe(true);

    const finalSlowRow = await getRowByModel(page, slowModel);
    const finalFastRow = await getRowByModel(page, fastModel);
    await expect(finalSlowRow).toContainText(TERMINAL_STATUS_RE, { timeout: 10_000 });
    await expect(finalFastRow).toContainText(TERMINAL_STATUS_RE, { timeout: 10_000 });
  } finally {
    await page.evaluate(() => {
      (window as { __maxxWsTest?: { setBlocked(next: boolean): void } }).__maxxWsTest?.setBlocked(
        false,
      );
    }).catch(() => {});

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
    if (projectId) {
      try {
        await adminAPI('DELETE', `/projects/${projectId}`, undefined, jwt);
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
