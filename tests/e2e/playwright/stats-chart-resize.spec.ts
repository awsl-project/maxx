/**
 * Playwright E2E Test: Stats Page - Chart Resize Error on Project Switch
 *
 * 使用方式：
 *   npx playwright test -c playwright.config.ts stats-chart-resize.spec.ts --project=e2e-chromium
 */
import http from 'node:http';
import os from 'node:os';
import path from 'node:path';

import { expect, test } from 'playwright/test';

import { BASE, PASS, USER, adminAPI, nextAnimationFrames } from './helpers';

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
                input_tokens: 150,
                output_tokens: 80,
                cache_creation_input_tokens: 10,
                cache_read_input_tokens: 20,
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

test('stats chart survives repeated zero-dimension resize cycles', async ({ page }, testInfo) => {
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
    console.log(`✅ Provider created: id=${provider.id}`);

    console.log('\n--- Setup: Create Projects ---');
    const ts = Date.now();
    const projectNames = [`Proj-A-${ts}`, `Proj-B-${ts}`, `Proj-C-${ts}`];
    const projects = [];
    for (let index = 0; index < 3; index += 1) {
      const project = await adminAPI(
        'POST',
        '/projects',
        {
          name: projectNames[index],
          slug: `proj-${String.fromCharCode(97 + index)}-${ts}`,
          enabledCustomRoutes: ['claude'],
        },
        jwt,
      );
      projects.push(project);
      projectIds.push(project.id);
      console.log(`✅ Project ${projectNames[index]} created: id=${project.id}`);
    }

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
    console.log('✅ Global route created');

    console.log('\n--- Setup: Create Tokens & Send Requests ---');
    for (let index = 0; index < 3; index += 1) {
      const tokenResult = await adminAPI(
        'POST',
        '/api-tokens',
        {
          name: `Token-${projectNames[index]}`,
          projectID: projects[index].id,
        },
        jwt,
      );
      tokenIds.push(tokenResult.apiToken.id);
      expect(tokenResult.token).toBeTruthy();

      if (index < 2) {
        for (let requestIndex = 0; requestIndex < 5; requestIndex += 1) {
          await sendClaudeRequest(tokenResult.token);
        }
        console.log(`✅ Sent 5 requests via ${projectNames[index]} token`);
      } else {
        console.log(`⏭️  Skipped requests for ${projectNames[index]} (empty project for testing)`);
      }
    }

    await expect.poll(async () => {
      const stats = await adminAPI('GET', '/usage-stats?limit=200', undefined, jwt);
      return Array.isArray(stats) ? stats.length : 0;
    }, { timeout: 20000 }).toBeGreaterThan(0);

    console.log('\n--- Step 1: Browser Login ---');
    await page.goto(BASE);
    await expect(page.locator('input[type="text"]')).toBeVisible({ timeout: 10000 });
    await page.fill('input[type="text"]', USER);
    await page.fill('input[type="password"]', PASS);
    await page.locator('button[type="submit"]').click();
    await expect(page.locator('body')).toContainText(/Dashboard|dashboard/, { timeout: 10000 });
    console.log('✅ Browser login success');

    const consoleErrors: string[] = [];
    const consoleWarnings: string[] = [];
    const pageErrors: string[] = [];
    page.on('console', (message) => {
      if (message.type() === 'error') consoleErrors.push(message.text());
      if (message.type() === 'warning') consoleWarnings.push(message.text());
    });
    page.on('pageerror', (error) => {
      pageErrors.push(error.message);
    });

    console.log('\n--- Step 2: Navigate to Stats ---');
    await page.goto(`${BASE}/stats`);
    await expect(page.locator('body')).toContainText(/Stats|stats|统计|Statistics/, { timeout: 15000 });
    console.log('✅ Stats page loaded');

    console.log('\n--- Step 3: Verify Chart Renders ---');
    const chartContainer = page.locator('.recharts-wrapper').first();
    await expect(chartContainer).toBeVisible({ timeout: 15000 });
    console.log('✅ Chart visible');

    console.log('\n--- Step 4: Trigger ResponsiveContainer Dimension Bug ---');
    for (let index = 0; index < 30; index += 1) {
      await page.evaluate(() => {
        const containers = document.querySelectorAll<HTMLElement>('.recharts-wrapper');
        containers.forEach((container) => {
          const parent = container.parentElement as HTMLElement | null;
          if (parent) {
            parent.style.width = '0px';
            parent.style.height = '0px';
            parent.style.overflow = 'hidden';
          }
        });
      });
      await nextAnimationFrames(page, 1);

      await page.evaluate(() => {
        const containers = document.querySelectorAll<HTMLElement>('.recharts-wrapper');
        containers.forEach((container) => {
          const parent = container.parentElement as HTMLElement | null;
          if (parent) {
            parent.style.width = '';
            parent.style.height = '';
            parent.style.overflow = '';
          }
        });
      });
      await nextAnimationFrames(page, 2);

      if (index % 5 === 0) {
        const chip = page.getByRole('button', { name: projectNames[index % 3], exact: true });
        if (await chip.count()) {
          await chip.click();
          await nextAnimationFrames(page, 2);
        }
      }
    }

    console.log('\n--- Step 4b: Verify Chart Recovery ---');
    await expect(chartContainer).toBeVisible({ timeout: 10000 });
    console.log('✅ Chart recovered after resize cycle');

    await page.waitForTimeout(1000);

    console.log('\n--- Step 5: Check Console Errors & Warnings ---');
    const allMessages = [...consoleErrors, ...consoleWarnings, ...pageErrors];
    const rechartsMessages = allMessages.filter(
      (message) => message.includes('width') && message.includes('height') && message.includes('greater than 0'),
    );

    console.log(`  Total console errors: ${consoleErrors.length}`);
    console.log(`  Total console warnings: ${consoleWarnings.length}`);
    console.log(`  Total page errors: ${pageErrors.length}`);
    console.log(`  Recharts dimension messages: ${rechartsMessages.length}`);
    expect(rechartsMessages).toHaveLength(0);
    console.log('✅ No Recharts dimension warnings — fix is working.');

    const screenshotPath = path.join(os.tmpdir(), 'stats-chart-resize-result.png');
    const screenshot = await page.screenshot({ path: screenshotPath });
    await testInfo.attach('stats-chart-resize-result', {
      body: screenshot,
      contentType: 'image/png',
    });
    console.log(`  Screenshot: ${screenshotPath}`);
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
