/**
 * Playwright E2E Test: Requests Page - Project Filter
 *
 * 使用方式：
 *   npx playwright test -c playwright.config.ts requests-project-filter.spec.ts --project=e2e-chromium
 */
import http from 'node:http';

import { expect, test } from 'playwright/test';

import { BASE, PASS, USER, adminAPI } from './helpers';

test.describe.configure({ mode: 'serial' });

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

          const model = parsed.model || 'claude-sonnet-4-20250514';
          res.writeHead(200, { 'Content-Type': 'application/json' });
          res.end(
            JSON.stringify({
              id: `msg_mock_${Date.now()}`,
              type: 'message',
              role: 'assistant',
              model,
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
      console.log(`✅ Mock Claude API server started on port ${address.port}`);
      resolve({ server, port: address.port });
    });
  });
}

async function sendClaudeRequest(apiToken: string, model = 'claude-sonnet-4-20250514') {
  const response = await fetch(`${BASE}/v1/messages`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'x-api-key': apiToken,
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

test('requests page can filter by project and persist selection', async ({ page }, testInfo) => {
  const mock = await startMockClaudeServer();
  let jwt: string | null = null;
  let providerId: number | null = null;
  let routeId: number | null = null;
  const projectIds: number[] = [];
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
    await adminAPI('PUT', '/settings/api_token_auth_enabled', { value: 'true' }, jwt);
    console.log('✅ API token auth enabled');

    console.log('\n--- Setup: Create Provider ---');
    const provider = await adminAPI(
      'POST',
      '/providers',
      {
        name: 'Mock Claude Provider',
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
    expect(providerId).toBeTruthy();
    console.log(`✅ Provider created: id=${provider.id}, name=${provider.name}`);

    console.log('\n--- Setup: Create Projects ---');
    const ts = Date.now();
    const projectAName = `Alpha-${ts}`;
    const projectBName = `Beta-${ts}`;

    const projectA = await adminAPI(
      'POST',
      '/projects',
      { name: projectAName, slug: `alpha-${ts}`, enabledCustomRoutes: ['claude'] },
      jwt,
    );
    const projectB = await adminAPI(
      'POST',
      '/projects',
      { name: projectBName, slug: `beta-${ts}`, enabledCustomRoutes: ['claude'] },
      jwt,
    );
    projectIds.push(projectA.id, projectB.id);
    console.log(`✅ Project A created: id=${projectA.id}, name=${projectA.name}`);
    console.log(`✅ Project B created: id=${projectB.id}, name=${projectB.name}`);

    console.log('\n--- Setup: Create Routes ---');
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
    console.log(`✅ Global route created: id=${route.id}`);

    console.log('\n--- Setup: Create API Tokens ---');
    const tokenAResult = await adminAPI(
      'POST',
      '/api-tokens',
      { name: 'Token for Alpha', description: 'Test token for Project Alpha', projectID: projectA.id },
      jwt,
    );
    const tokenBResult = await adminAPI(
      'POST',
      '/api-tokens',
      { name: 'Token for Beta', description: 'Test token for Project Beta', projectID: projectB.id },
      jwt,
    );
    tokenIds.push(tokenAResult.apiToken.id, tokenBResult.apiToken.id);
    const tokenA = tokenAResult.token as string;
    const tokenB = tokenBResult.token as string;
    expect(tokenA).toBeTruthy();
    expect(tokenB).toBeTruthy();
    console.log(`✅ Token A created: prefix=${tokenAResult.apiToken.tokenPrefix}`);
    console.log(`✅ Token B created: prefix=${tokenBResult.apiToken.tokenPrefix}`);

    console.log('\n--- Setup: Send Proxy Requests ---');
    for (let i = 0; i < 3; i += 1) {
      expect((await sendClaudeRequest(tokenA)).id).toBeTruthy();
    }
    console.log('✅ Sent 3 requests via Project Alpha token');

    for (let i = 0; i < 2; i += 1) {
      expect((await sendClaudeRequest(tokenB)).id).toBeTruthy();
    }
    console.log('✅ Sent 2 requests via Project Beta token');

    await expect.poll(async () => {
      const requests = await adminAPI('GET', '/requests?limit=100', undefined, jwt);
      const alphaCount = requests.items?.filter((item: any) => item.projectID === projectA.id).length ?? 0;
      const betaCount = requests.items?.filter((item: any) => item.projectID === projectB.id).length ?? 0;
      return { alphaCount, betaCount };
    }, { timeout: 15000 }).toEqual({ alphaCount: 3, betaCount: 2 });

    console.log('\n--- Step 1: Browser Login ---');
    await page.goto(BASE);
    await expect(page.locator('input[type="text"]')).toBeVisible({ timeout: 10000 });
    await page.fill('input[type="text"]', USER);
    await page.fill('input[type="password"]', PASS);
    await page.locator('button[type="submit"]').click();
    await expect(page.locator('body')).toContainText(/Dashboard|dashboard/, { timeout: 10000 });
    console.log('✅ Browser login success');

    console.log('\n--- Step 2: Navigate to Requests ---');
    await page.goto(`${BASE}/requests`);
    await expect(page.locator('body')).toContainText('total requests', { timeout: 15000 });
    console.log('✅ Requests page loaded');

    const filterModeSelect = page.locator('main [role="combobox"]').first();

    console.log('\n--- Step 3: Verify Project Filter Option ---');
    let hasProjectOption = false;
    for (let attempt = 0; attempt < 10; attempt += 1) {
      await filterModeSelect.click();
      const projectOption = page.locator('[role="option"]').filter({ hasText: /Project|项目/ }).first();
      if (await projectOption.count()) {
        hasProjectOption = true;
        break;
      }
      await page.keyboard.press('Escape');
      await page.waitForTimeout(200);
    }
    expect(hasProjectOption).toBe(true);
    console.log('✅ Project option exists in filter mode selector');

    console.log('\n--- Step 4: Select Project Filter Mode ---');
    await page.locator('[role="option"]').filter({ hasText: /Project|项目/ }).first().click();
    await expect(filterModeSelect).toContainText(/Project|项目/);
    console.log('✅ Filter mode switched to Project');

    console.log('\n--- Step 5: Select Project Alpha ---');
    const projectFilterSelect = page.locator('main [role="combobox"]').nth(1);
    await projectFilterSelect.click();
    await expect(page.locator('[role="option"]').filter({ hasText: /All Projects|全部项目/ }).first()).toBeVisible();
    await expect(page.locator('[role="option"]').filter({ hasText: projectAName }).first()).toBeVisible();
    await expect(page.locator('[role="option"]').filter({ hasText: projectBName }).first()).toBeVisible();
    console.log('✅ Project dropdown shows all projects');

    await page.locator('[role="option"]').filter({ hasText: projectAName }).first().click();
    await expect.poll(async () => /3 total/.test((await page.textContent('body')) ?? ''), { timeout: 10000 }).toBe(true);
    console.log('✅ Project Alpha filter: 3 requests shown');

    console.log('\n--- Step 6: Switch to Project Beta ---');
    await projectFilterSelect.click();
    await page.locator('[role="option"]').filter({ hasText: projectBName }).first().click();
    await expect.poll(async () => /2 total/.test((await page.textContent('body')) ?? ''), { timeout: 10000 }).toBe(true);
    console.log('✅ Project Beta filter: 2 requests shown');

    console.log('\n--- Step 7: Clear Project Filter ---');
    await projectFilterSelect.click();
    await page.locator('[role="option"]').filter({ hasText: /All Projects|全部项目/ }).first().click();
    await expect.poll(async () => {
      const match = ((await page.textContent('body')) ?? '').match(/(\d+) total/);
      return match ? Number.parseInt(match[1], 10) : 0;
    }, { timeout: 10000 }).toBeGreaterThan(0);
    console.log('✅ All Projects filter restored');

    console.log('\n--- Step 8: Switch to Token Filter Mode ---');
    await filterModeSelect.click();
    await page.locator('[role="option"]').filter({ hasText: /^Token$|^令牌$/ }).first().click();
    await expect(filterModeSelect).toContainText(/Token|令牌/);
    console.log('✅ Switched back to Token filter mode');

    console.log('\n--- Step 9: Test Persistence ---');
    await filterModeSelect.click();
    await page.locator('[role="option"]').filter({ hasText: /Project|项目/ }).first().click();
    await projectFilterSelect.click();
    await page.locator('[role="option"]').filter({ hasText: projectAName }).first().click();
    await page.reload({ waitUntil: 'networkidle' });
    await expect(page.locator('main [role="combobox"]').first()).toContainText(/Project|项目/);
    await expect(page.locator('main [role="combobox"]').nth(1)).toContainText(projectAName);
    console.log('✅ Filter state persisted across page reload');

    const screenshot = await page.screenshot({ path: '/tmp/requests-project-filter-result.png' });
    await testInfo.attach('requests-project-filter-result', {
      body: screenshot,
      contentType: 'image/png',
    });
    console.log('  Screenshot: /tmp/requests-project-filter-result.png');
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
    for (const id of projectIds.reverse()) {
      try {
        await adminAPI('DELETE', `/projects/${id}`, undefined, jwt ?? undefined);
      } catch {}
    }
    if (providerId) {
      try {
        await adminAPI('DELETE', `/providers/${providerId}`, undefined, jwt ?? undefined);
      } catch {}
    }
    await new Promise((resolve) => mock.server.close(() => resolve(undefined)));
  }
});
