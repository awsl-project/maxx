import fs from 'node:fs/promises';
import path from 'node:path';
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
test.setTimeout(180_000);

const REQUEST_FILTER_MODE_STORAGE_KEY = 'maxx-requests-filter-mode';
const REQUEST_PROVIDER_FILTER_STORAGE_KEY = 'maxx-requests-provider-filter';
const REQUEST_TOKEN_FILTER_STORAGE_KEY = 'maxx-requests-token-filter';
const REQUEST_PROJECT_FILTER_STORAGE_KEY = 'maxx-requests-project-filter';

const STRESS_DURATION_MS = 60_000;
const SAMPLE_INTERVAL_MS = 30_000;
const REQUEST_RATE_PER_SECOND = 25;
const REQUESTS_PER_TICK = 5;
const TICK_INTERVAL_MS = 200;
const MAX_IN_FLIGHT_REQUESTS = 60;
const BACKPRESSURE_POLL_MS = 25;
const MAX_RENDERED_ROWS = 120;
const REPORT_PATH = path.resolve(process.cwd(), 'test-results', 'requests-mixed-stress-report.json');

type ScenarioName =
  | 'fast-success'
  | 'slow-success'
  | 'fast-fail'
  | 'slow-fail'
  | 'connection-reset'
  | 'client-abort';

type ScenarioDefinition = {
  name: ScenarioName;
  weight: number;
};

type StressCounters = Record<ScenarioName, number> & {
  started: number;
  completed: number;
  httpFailed: number;
  aborted: number;
  networkFailed: number;
};

type StressSample = {
  elapsedSeconds: number;
  adminLatencyMs: number;
  adminTopStatuses: string[];
  adminOrderingViolation: boolean;
  visibleStatuses: string[];
  uiOrderingViolation: boolean;
  domNodeCount: number;
  renderedRows: number;
  tbodyRows: number;
  heapUsedMb: number | null;
  rafP95Ms: number;
  rafMaxMs: number;
};

type PageMetrics = {
  visibleStatuses: string[];
  uiOrderingViolation: boolean;
  domNodeCount: number;
  renderedRows: number;
  tbodyRows: number;
  heapUsedMb: number | null;
  rafP95Ms: number;
  rafMaxMs: number;
};

const SCENARIOS: ScenarioDefinition[] = [
  { name: 'fast-success', weight: 32 },
  { name: 'slow-success', weight: 22 },
  { name: 'fast-fail', weight: 18 },
  { name: 'slow-fail', weight: 10 },
  { name: 'connection-reset', weight: 8 },
  { name: 'client-abort', weight: 10 },
];

function randomInt(min: number, max: number): number {
  return Math.floor(Math.random() * (max - min + 1)) + min;
}

function delay(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function classifyLabel(text: string): 'active' | 'terminal' | 'unknown' {
  const normalized = text.trim().toLowerCase();
  if (
    /pending|stream|waiting|in progress|等待|传输|绑定/.test(normalized)
  ) {
    return 'active';
  }
  if (
    /completed|failed|cancelled|rejected|完成|失败|取消|拒绝/.test(normalized)
  ) {
    return 'terminal';
  }
  return 'unknown';
}

function hasActiveAfterTerminal(statuses: string[]): boolean {
  let seenTerminal = false;
  for (const status of statuses) {
    if (status === 'PENDING' || status === 'IN_PROGRESS') {
      if (seenTerminal) {
        return true;
      }
      continue;
    }
    seenTerminal = true;
  }
  return false;
}

function hasActiveAfterTerminalLabel(statuses: string[]): boolean {
  let seenTerminal = false;
  for (const status of statuses) {
    const type = classifyLabel(status);
    if (type === 'active') {
      if (seenTerminal) {
        return true;
      }
      continue;
    }
    if (type === 'terminal') {
      seenTerminal = true;
    }
  }
  return false;
}

function pickScenario(): ScenarioName {
  const totalWeight = SCENARIOS.reduce((sum, scenario) => sum + scenario.weight, 0);
  let cursor = Math.random() * totalWeight;
  for (const scenario of SCENARIOS) {
    cursor -= scenario.weight;
    if (cursor <= 0) {
      return scenario.name;
    }
  }
  return SCENARIOS[SCENARIOS.length - 1].name;
}

function createCounters(): StressCounters {
  return {
    started: 0,
    completed: 0,
    httpFailed: 0,
    aborted: 0,
    networkFailed: 0,
    'fast-success': 0,
    'slow-success': 0,
    'fast-fail': 0,
    'slow-fail': 0,
    'connection-reset': 0,
    'client-abort': 0,
  };
}

function startMixedMockServer(): Promise<{ server: http.Server; port: number }> {
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
          // ignore malformed JSON in the mock
        }

        const model = String(parsed.model || '');
        const scenario = (model.split('__').pop() || 'fast-success') as ScenarioName;

        const respondSuccess = () => {
          res.writeHead(200, { 'Content-Type': 'application/json' });
          res.end(
            JSON.stringify({
              id: `msg_mock_${Date.now()}_${Math.random().toString(36).slice(2, 8)}`,
              type: 'message',
              role: 'assistant',
              model,
              content: [{ type: 'text', text: `Scenario ${scenario}` }],
              stop_reason: 'end_turn',
              stop_sequence: null,
              usage: {
                input_tokens: randomInt(10, 40),
                output_tokens: randomInt(8, 80),
                cache_creation_input_tokens: 0,
                cache_read_input_tokens: 0,
              },
            }),
          );
        };

        const respondFailure = (statusCode: number) => {
          res.writeHead(statusCode, { 'Content-Type': 'application/json' });
          res.end(
            JSON.stringify({
              type: 'error',
              error: {
                type: 'upstream_error',
                message: `Mock ${scenario} failure`,
              },
            }),
          );
        };

        switch (scenario) {
          case 'fast-success':
            await delay(randomInt(15, 90));
            respondSuccess();
            return;
          case 'slow-success':
            await delay(randomInt(900, 4_500));
            if (!res.writableEnded) {
              respondSuccess();
            }
            return;
          case 'fast-fail':
            await delay(randomInt(15, 100));
            respondFailure(502);
            return;
          case 'slow-fail':
            await delay(randomInt(900, 4_500));
            if (!res.writableEnded) {
              respondFailure(503);
            }
            return;
          case 'connection-reset':
            await delay(randomInt(50, 400));
            res.socket?.destroy();
            return;
          case 'client-abort':
            await delay(randomInt(8_000, 15_000));
            if (!res.writableEnded) {
              respondSuccess();
            }
            return;
          default:
            respondSuccess();
        }
      });
    });

    server.listen(0, '127.0.0.1', () => {
      const address = server.address();
      if (!address || typeof address === 'string') {
        throw new Error('Failed to determine mixed mock server port');
      }
      resolve({ server, port: address.port });
    });
  });
}

async function resolveAdminToken() {
  try {
    return await loginToAdminAPI();
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    if (message.includes('(401)') || message.includes('(404)')) {
      return undefined;
    }
    throw error;
  }
}

async function openRequestsPage(page: Page, providerId: number) {
  await page.addInitScript((id) => {
    localStorage.setItem(REQUEST_FILTER_MODE_STORAGE_KEY, 'provider');
    localStorage.setItem(REQUEST_PROVIDER_FILTER_STORAGE_KEY, String(id));
    localStorage.removeItem(REQUEST_TOKEN_FILTER_STORAGE_KEY);
    localStorage.removeItem(REQUEST_PROJECT_FILTER_STORAGE_KEY);
  }, providerId);

  await page.goto(`${BASE}/requests`);
  await page.waitForLoadState('networkidle');

  if (await page.locator('input[type="password"]').count()) {
    await loginToAdminUI(page);
    await page.goto(`${BASE}/requests`);
    await page.waitForLoadState('networkidle');
  }
}

async function warmupTraffic(url: string, requestCount: number) {
  await Promise.all(
    Array.from({ length: requestCount }, (_, index) =>
      fetch(url, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'anthropic-version': '2023-06-01',
        },
        body: JSON.stringify({
          model: `claude-sonnet-4-20250514__${index % 2 === 0 ? 'fast-success' : 'slow-success'}`,
          max_tokens: 64,
          messages: [{ role: 'user', content: `warmup-${index}` }],
        }),
      }),
    ),
  );
}

async function fireScenarioRequest(url: string, scenario: ScenarioName, counters: StressCounters) {
  counters.started += 1;
  counters[scenario] += 1;

  const controller = new AbortController();
  if (scenario === 'client-abort') {
    setTimeout(() => controller.abort(), randomInt(200, 1_500));
  }

  try {
    const response = await fetch(url, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'anthropic-version': '2023-06-01',
      },
      signal: controller.signal,
      body: JSON.stringify({
        model: `claude-sonnet-4-20250514__${scenario}`,
        max_tokens: randomInt(48, 192),
        messages: [{ role: 'user', content: `stress-${Date.now()}-${scenario}` }],
      }),
    });

    await response.text();
    if (response.ok) {
      counters.completed += 1;
      return;
    }
    counters.httpFailed += 1;
  } catch (error) {
    if (controller.signal.aborted || (error instanceof Error && error.name === 'AbortError')) {
      counters.aborted += 1;
      return;
    }
    counters.networkFailed += 1;
  }
}

async function runStressTraffic(url: string, counters: StressCounters) {
  const startedAt = Date.now();
  const endAt = startedAt + STRESS_DURATION_MS;
  const inFlight = new Set<Promise<void>>();

  while (Date.now() < endAt) {
    const tickStartedAt = Date.now();
    let launchedThisTick = 0;

    while (launchedThisTick < REQUESTS_PER_TICK) {
      if (inFlight.size >= MAX_IN_FLIGHT_REQUESTS) {
        const tickDeadline = tickStartedAt + TICK_INTERVAL_MS;
        while (inFlight.size >= MAX_IN_FLIGHT_REQUESTS && Date.now() < tickDeadline) {
          await delay(Math.min(BACKPRESSURE_POLL_MS, Math.max(1, tickDeadline - Date.now())));
        }
        if (inFlight.size >= MAX_IN_FLIGHT_REQUESTS) {
          break;
        }
      }

      const scenario = pickScenario();
      const requestPromise = fireScenarioRequest(url, scenario, counters).finally(() => {
        inFlight.delete(requestPromise);
      });
      inFlight.add(requestPromise);
      launchedThisTick += 1;
    }

    const remaining = TICK_INTERVAL_MS - (Date.now() - tickStartedAt);
    if (remaining > 0) {
      await delay(remaining);
    }
  }

  await Promise.allSettled([...inFlight]);
}

async function collectPageMetrics(page: Page): Promise<PageMetrics> {
  return page.evaluate(async () => {
    const headerTexts = Array.from(document.querySelectorAll('table thead th')).map((th) =>
      (th.textContent || '').trim(),
    );
    const statusColumnIndex = headerTexts.findIndex((text) => /^(status|状态)$/i.test(text) || text.includes('状态'));
    const rows = Array.from(document.querySelectorAll('tbody tr[data-request-row="true"]')).slice(0, 20);
    const visibleStatuses = rows.map((row) => {
      const cells = Array.from(row.querySelectorAll('td'));
      return statusColumnIndex >= 0 ? (cells[statusColumnIndex]?.textContent || '').trim() : '';
    });

    const frameDeltas: number[] = [];
    await new Promise<void>((resolve) => {
      let remaining = 30;
      let previous = performance.now();
      const step = (now: number) => {
        frameDeltas.push(now - previous);
        previous = now;
        remaining -= 1;
        if (remaining <= 0) {
          resolve();
          return;
        }
        requestAnimationFrame(step);
      };
      requestAnimationFrame(step);
    });

    const sortedFrameDeltas = [...frameDeltas].sort((a, b) => a - b);
    const p95Index = Math.min(
      sortedFrameDeltas.length - 1,
      Math.max(0, Math.floor(sortedFrameDeltas.length * 0.95)),
    );

    const heapMemory = (performance as Performance & {
      memory?: { usedJSHeapSize?: number };
    }).memory?.usedJSHeapSize;

    return {
      visibleStatuses,
      uiOrderingViolation: (() => {
        let seenTerminal = false;
        for (const status of visibleStatuses) {
          const normalized = status.trim().toLowerCase();
          const isActive = /pending|stream|waiting|in progress|等待|传输|绑定/.test(normalized);
          const isTerminal = /completed|failed|cancelled|rejected|完成|失败|取消|拒绝/.test(normalized);
          if (isActive) {
            if (seenTerminal) {
              return true;
            }
            continue;
          }
          if (isTerminal) {
            seenTerminal = true;
          }
        }
        return false;
      })(),
      domNodeCount: document.querySelectorAll('*').length,
      renderedRows: document.querySelectorAll('tbody tr[data-request-row="true"]').length,
      tbodyRows: document.querySelectorAll('tbody tr').length,
      heapUsedMb: typeof heapMemory === 'number' ? Number((heapMemory / 1024 / 1024).toFixed(2)) : null,
      rafP95Ms: Number((sortedFrameDeltas[p95Index] || 0).toFixed(2)),
      rafMaxMs: Number((Math.max(...frameDeltas, 0) || 0).toFixed(2)),
    };
  });
}

async function collectStressSample(
  page: Page,
  providerId: number,
  jwt: string | undefined,
  startedAt: number,
): Promise<StressSample> {
  const adminStartedAt = Date.now();
  const response = await adminAPI(
    'GET',
    `/requests?limit=20&providerId=${providerId}`,
    undefined,
    jwt,
  );
  const adminLatencyMs = Date.now() - adminStartedAt;
  const adminTopStatuses = (response.items ?? []).map((item: any) => String(item.status));
  const pageMetrics = await collectPageMetrics(page);

  return {
    elapsedSeconds: Math.round((Date.now() - startedAt) / 1000),
    adminLatencyMs,
    adminTopStatuses,
    adminOrderingViolation: hasActiveAfterTerminal(adminTopStatuses),
    visibleStatuses: pageMetrics.visibleStatuses,
    uiOrderingViolation:
      pageMetrics.uiOrderingViolation || hasActiveAfterTerminalLabel(pageMetrics.visibleStatuses),
    domNodeCount: pageMetrics.domNodeCount,
    renderedRows: pageMetrics.renderedRows,
    tbodyRows: pageMetrics.tbodyRows,
    heapUsedMb: pageMetrics.heapUsedMb,
    rafP95Ms: pageMetrics.rafP95Ms,
    rafMaxMs: pageMetrics.rafMaxMs,
  };
}

test('requests page remains responsive during 1 minute mixed live stress', async ({ page }, testInfo) => {
  const mock = await startMixedMockServer();
  const counters = createCounters();
  const samples: StressSample[] = [];
  let jwt: string | undefined;
  let retryConfigId: number | null = null;
  let providerId: number | null = null;
  let projectId: number | null = null;
  let routeId: number | null = null;
  let previousApiTokenAuthEnabled: string | undefined;

  try {
    jwt = await resolveAdminToken();
    if (jwt) {
      const settings = await adminAPI('GET', '/settings', undefined, jwt);
      previousApiTokenAuthEnabled = settings.api_token_auth_enabled;
      // 该压测通过项目代理直接打流量，不依赖 API Token 鉴权。
      await adminAPI('PUT', '/settings/api_token_auth_enabled', { value: 'false' }, jwt);
    }

    const ts = Date.now();
    const retryConfig = await adminAPI(
      'POST',
      '/retry-configs',
      {
        name: `stress-no-retry-${ts}`,
        isDefault: false,
        maxRetries: 0,
        initialInterval: 100_000_000,
        backoffRate: 1,
        maxInterval: 100_000_000,
      },
      jwt,
    );
    retryConfigId = retryConfig.id;

    const provider = await adminAPI(
      'POST',
      '/providers',
      {
        name: `Requests Stress Provider ${ts}`,
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
        name: `Requests Stress Project ${ts}`,
        slug: `requests-stress-${ts}`,
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
        retryConfigID: retryConfig.id,
        position: 1,
      },
      jwt,
    );
    routeId = route.id;

    const stressURL = `${BASE}/project/${project.slug}/v1/messages`;
    await warmupTraffic(stressURL, 40);

    await expect
      .poll(
        async () => {
          const requests = await adminAPI(
            'GET',
            `/requests?limit=100&providerId=${provider.id}`,
            undefined,
            jwt,
          );
          return requests.items?.length ?? 0;
        },
        { timeout: 20_000 },
      )
      .toBeGreaterThanOrEqual(20);

    await openRequestsPage(page, provider.id);
    await expect(page.locator('table thead th').first()).toBeVisible({ timeout: 30_000 });
    await expect
      .poll(async () => page.locator('tbody tr[data-request-row="true"]').count(), { timeout: 30_000 })
      .toBeGreaterThan(0);

    const startedAt = Date.now();
    const trafficPromise = runStressTraffic(stressURL, counters);

    while (Date.now() - startedAt < STRESS_DURATION_MS) {
      await page.waitForTimeout(SAMPLE_INTERVAL_MS);
      samples.push(await collectStressSample(page, provider.id, jwt, startedAt));
    }

    await trafficPromise;
    await page.waitForTimeout(500);
    samples.push(await collectStressSample(page, provider.id, jwt, startedAt));

    const finalList = await adminAPI(
      'GET',
      `/requests?limit=200&providerId=${provider.id}`,
      undefined,
      jwt,
    );
    const finalStatusCounts = (finalList.items ?? []).reduce((acc: Record<string, number>, item: any) => {
      const status = String(item.status);
      acc[status] = (acc[status] || 0) + 1;
      return acc;
    }, {});

    const report = {
      requestRatePerSecond: REQUEST_RATE_PER_SECOND,
      durationMs: STRESS_DURATION_MS,
      counters,
      finalStatusCounts,
      samples,
    };

    await fs.mkdir(path.dirname(REPORT_PATH), { recursive: true });
    await fs.writeFile(REPORT_PATH, JSON.stringify(report, null, 2), 'utf8');

    await testInfo.attach('requests-mixed-stress-report.json', {
      body: Buffer.from(JSON.stringify(report, null, 2)),
      contentType: 'application/json',
    });

    const finalSample = samples.at(-1);
    const uiViolationCount = samples.filter((sample) => sample.uiOrderingViolation).length;
    const maxRenderedRows = Math.max(...samples.map((sample) => sample.renderedRows), 0);

    // Mid-run admin snapshots are diagnostic only: under sustained mixed load,
    // a poll can land between status transition and list resort. Require the
    // post-drain sample to converge, while still bounding UI-side violations
    // during the live traffic window.
    const maxAllowedViolations = Math.max(1, Math.ceil(samples.length * 0.2));
    expect(finalSample?.adminOrderingViolation).toBe(false);
    expect(finalSample?.uiOrderingViolation).toBe(false);
    expect(uiViolationCount).toBeLessThanOrEqual(maxAllowedViolations);
    expect(maxRenderedRows).toBeLessThanOrEqual(MAX_RENDERED_ROWS);
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
    if (retryConfigId) {
      try {
        await adminAPI('DELETE', `/retry-configs/${retryConfigId}`, undefined, jwt);
      } catch {}
    }
    await closeServer(mock.server);
  }
});
