import { expect, test, type Page, type Route } from '@playwright/test';

const now = new Date().toISOString();

type Call = {
  time: string;
  method: string;
  path: string;
  status: number;
  note: string;
  body?: unknown;
};

async function json(route: Route, body: unknown, status = 200, note = '') {
  await route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) });
  return { status, note };
}

function providerFixture() {
  return [
    {
      id: 42,
      createdAt: now,
      updatedAt: now,
      tenantID: 1,
      name: 'Mock Grok OAuth Provider',
      type: 'grok',
      isEnabled: true,
      maxConcurrency: 2,
      supportedClientTypes: ['openai'],
      config: {
        grok: {
          type: 'xai',
          authKind: 'oauth',
          accessToken: '',
          refreshToken: '',
          idToken: '',
        },
      },
    },
    {
      id: 99,
      createdAt: now,
      updatedAt: now,
      tenantID: 1,
      name: 'Unsupported Claude Provider',
      type: 'claude',
      isEnabled: true,
      supportedClientTypes: ['claude'],
      config: {},
    },
  ];
}

async function installTestFieldMocks(page: Page, calls: Call[]) {
  await page.addInitScript(() => {
    localStorage.setItem('maxx-admin-token', 'mock-token');
    localStorage.setItem('maxx-ui-language', 'zh');
  });

  await page.route('**/api/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname;
    const method = request.method();
    const body = request.postData() ? request.postDataJSON() : undefined;
    let outcome: { status: number; note: string };

    if (path === '/api/settings' || path === '/api/admin/settings') {
      outcome = await json(
        route,
        {
          api_token_auth_enabled: 'true',
          force_project_binding: 'false',
          ui_multitenant_enabled: 'false',
          ui_test_field_tab_enabled: 'true',
        },
        200,
        'settings expose test field tab',
      );
    } else if (path === '/api/admin/auth/status') {
      outcome = await json(
        route,
        {
          authEnabled: true,
          user: { id: 1, username: 'admin', tenantID: 1, role: 'admin' },
        },
        200,
        'authenticated admin',
      );
    } else if (path === '/api/admin/proxy-status' || path === '/api/proxy-status') {
      outcome = await json(
        route,
        { running: true, address: '127.0.0.1', port: 9880, version: 'e2e-test-field' },
        200,
        'proxy visible in layout',
      );
    } else if (path === '/api/admin/providers' || path === '/api/providers') {
      outcome = await json(route, providerFixture(), 200, 'provider list includes grok');
    } else if (path === '/api/admin/test-field/model-benchmark-jobs' && method === 'POST') {
      expect(body).toMatchObject({
        providerIDs: [42],
        concurrency: 1,
        timeoutMs: 5000,
        minModelsPerProvider: 200,
        reuseCachedModelLists: true,
        reuseCachedResults: true,
      });
      outcome = await json(route, { jobID: 'mock-grok-job-1' }, 202, 'benchmark job accepted');
    } else if (path === '/api/admin/test-field/model-benchmark-jobs/mock-grok-job-1' && method === 'GET') {
      outcome = await json(
        route,
        {
          jobID: 'mock-grok-job-1',
          status: 'completed',
          prompt: '端到端回归：请返回 mock-grok-ok',
          concurrency: 1,
          timeoutMs: 5000,
          minModelsPerProvider: 200,
          startedAt: now,
          finishedAt: new Date(Date.now() + 12).toISOString(),
          providers: [
            {
              providerID: 42,
              providerName: 'Mock Grok OAuth Provider',
              providerType: 'grok',
              available: true,
              modelCount: 102,
              testedCount: 102,
              cachedModels: false,
            },
          ],
          results: [
            {
              providerID: 42,
              providerName: 'Mock Grok OAuth Provider',
              providerType: 'grok',
              model: 'grok-4',
              available: true,
              durationMs: 37,
              statusCode: 200,
              response: 'mock-grok-ok: client /provider/42/v1/chat/completions -> xAI /responses',
              startedAt: now,
              finishedAt: new Date(Date.now() + 37).toISOString(),
            },
            {
              providerID: 42,
              providerName: 'Mock Grok OAuth Provider',
              providerType: 'grok',
              model: 'grok-latest',
              available: true,
              durationMs: 42,
              statusCode: 200,
              response: 'mock-grok-latest-ok via xAI /responses',
              startedAt: now,
              finishedAt: new Date(Date.now() + 42).toISOString(),
            },
          ],
          totalTargets: 102,
          completedTargets: 102,
          cachedResultCount: 0,
        },
        200,
        'benchmark completed with grok results',
      );
    } else {
      outcome = await json(route, {}, 200, 'fallback empty mock');
    }

    calls.push({ time: new Date().toISOString(), method, path, status: outcome.status, note: outcome.note, body });
  });
}

async function attachMockEvidence(page: Page, calls: Call[]) {
  await page.evaluate((mockCalls) => {
    document.querySelector('[data-e2e-mock-ledger]')?.remove();
    const el = document.createElement('div');
    el.dataset.e2eMockLedger = 'true';
    el.style.cssText = [
      'position:fixed',
      'right:16px',
      'bottom:16px',
      'z-index:99999',
      'width:520px',
      'max-height:340px',
      'overflow:auto',
      'padding:12px',
      'border-radius:12px',
      'background:rgba(2,6,23,0.92)',
      'color:white',
      'font:12px/1.45 ui-monospace,SFMono-Regular,Menlo,monospace',
      'box-shadow:0 12px 40px rgba(0,0,0,0.35)',
      'white-space:pre-wrap',
    ].join(';');
    el.textContent = `Test Field Grok E2E Mock Ledger\n${JSON.stringify(mockCalls, null, 2)}`;
    document.body.appendChild(el);
  }, calls);
}

test.use({ viewport: { width: 1440, height: 1000 }, locale: 'zh-CN' });

test('test field runs a Grok provider benchmark without blank-screening', async ({ page }, testInfo) => {
  const calls: Call[] = [];
  await installTestFieldMocks(page, calls);

  await page.goto('/test-field');
  await expect(page.getByRole('heading', { name: /测试场|Test Field/ })).toBeVisible();
  await expect(page.getByText('Mock Grok OAuth Provider')).toBeHidden();

  const providerInput = page.getByRole('combobox').first();
  await providerInput.click();
  await page.getByRole('option', { name: /Mock Grok OAuth Provider · grok · #42/ }).click();
  await page.getByRole('button', { name: /新增|Add/ }).click();
  await expect(page.getByText('Mock Grok OAuth Provider · grok · #42')).toBeVisible();
  await expect(page.getByRole('button', { name: /Run|运行|开始测试|开始/ })).toBeEnabled();

  await page.getByLabel(/^测试问题$/).fill('端到端回归：请返回 mock-grok-ok');
  await page.getByLabel(/^并发数$/).fill('1');
  await page.getByLabel(/^单模型超时 ms$/).fill('5000');
  await page.getByLabel(/^每个提供商最少测试模型数$/).fill('200');

  await attachMockEvidence(page, calls);
  await page.screenshot({ path: testInfo.outputPath('01-before-run-grok-provider-selected.png'), fullPage: true });

  await page.getByRole('button', { name: /Run|运行|开始测试|开始/ }).click();

  await expect(page.getByText(/mock-grok-ok: client \/provider\/42\/v1\/chat\/completions -> xAI \/responses/)).toBeVisible({
    timeout: 10_000,
  });
  await expect(page.getByRole('cell', { name: 'grok-4', exact: true })).toBeVisible();
  await expect(page.getByRole('cell', { name: 'grok-latest', exact: true })).toBeVisible();
  await expect(page.getByText(/102\/102|completed: 102|已完成/)).toBeVisible();
  await expect(page.getByText(/计划测试 102 \/ 发现 102 个模型/)).toBeVisible();

  expect(calls.some((call) => call.path === '/api/admin/test-field/model-benchmark-jobs' && call.method === 'POST')).toBe(true);
  expect(calls.some((call) => call.path === '/api/admin/test-field/model-benchmark-jobs/mock-grok-job-1' && call.method === 'GET')).toBe(true);

  await attachMockEvidence(page, calls);
  await page.screenshot({ path: testInfo.outputPath('02-after-run-grok-results-and-mock-ledger.png'), fullPage: true });
});
