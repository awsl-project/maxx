/**
 * Playwright E2E Test: Stats Page - Chart Resize Error on Project Switch
 *
 * 复现 Issue #220：
 * 在「统计」页面来回切换项目时，Recharts 图表渲染报错：
 *   "The width(-1) and height(-1) of chart should be greater than 0"
 *
 * 测试流程：
 * 1. 启动 mock Claude API 服务
 * 2. 创建 provider、多个 project、route、API token
 * 3. 发送代理请求生成统计数据
 * 4. 浏览器登录，进入统计页面
 * 5. 快速来回切换项目，监听 console 报错
 *
 * 使用方式：
 *   node test-stats-chart-resize.mjs [base_url] [username] [password]
 */
import http from 'node:http';
import os from 'node:os';
import path from 'node:path';
import { chromium } from 'playwright';

const BASE = process.argv[2] || 'http://localhost:9880';
const USER = process.argv[3] || 'admin';
const PASS = process.argv[4] || 'test123';
const HEADED = !!process.env.HEADED;

let exitCode = 0;
let mockServer = null;
let browser = null;

function assert(condition, msg) {
  if (!condition) {
    console.error(`❌ ASSERTION FAILED: ${msg}`);
    exitCode = 1;
    throw new Error(msg);
  }
}

// ===== Mock Claude API Server =====
function startMockClaudeServer() {
  return new Promise((resolve) => {
    const server = http.createServer((req, res) => {
      if (req.method === 'POST' && req.url.includes('/v1/messages')) {
        let body = '';
        req.on('data', (chunk) => (body += chunk));
        req.on('end', () => {
          let parsed = {};
          try {
            parsed = JSON.parse(body);
          } catch {
            // Ignore malformed JSON; mock server will use defaults
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
      const port = server.address().port;
      console.log(`✅ Mock Claude API server started on port ${port}`);
      resolve({ server, port });
    });
  });
}

// ===== Admin API Helper =====
async function adminAPI(method, path, body, token) {
  const url = `${BASE}/api/admin${path}`;
  const headers = { 'Content-Type': 'application/json' };
  if (token) headers['Authorization'] = `Bearer ${token}`;

  const res = await fetch(url, {
    method,
    headers,
    body: body ? JSON.stringify(body) : undefined,
  });

  const text = await res.text();
  let json;
  try {
    json = JSON.parse(text);
  } catch {
    json = text;
  }

  if (!res.ok) {
    throw new Error(`Admin API ${method} ${path} failed (${res.status}): ${text}`);
  }
  return json;
}

// ===== Proxy Request Helper =====
async function sendClaudeRequest(apiToken, model = 'claude-sonnet-4-20250514') {
  const res = await fetch(`${BASE}/v1/messages`, {
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

  const text = await res.text();
  if (!res.ok) {
    throw new Error(`Proxy request failed (${res.status}): ${text}`);
  }
  return JSON.parse(text);
}

// ===== Main Test =====
(async () => {
  // --- Setup: Start mock server ---
  console.log('\n--- Setup: Mock Claude API Server ---');
  const mock = await startMockClaudeServer();
  mockServer = mock.server;
  const mockBaseURL = `http://127.0.0.1:${mock.port}`;

  // --- Setup: Admin login ---
  console.log('\n--- Setup: Admin Login ---');
  const loginResp = await adminAPI('POST', '/auth/login', {
    username: USER,
    password: PASS,
  });
  assert(loginResp.token, 'Should receive JWT token');
  const jwt = loginResp.token;
  console.log('✅ Admin login success');

  // --- Setup: Enable API Token Auth ---
  await adminAPI('PUT', '/settings/api_token_auth_enabled', { value: 'true' }, jwt);
  console.log('✅ API token auth enabled');

  // --- Setup: Create Provider ---
  console.log('\n--- Setup: Create Provider ---');
  const provider = await adminAPI(
    'POST',
    '/providers',
    {
      name: 'Mock Claude Provider',
      type: 'custom',
      config: {
        custom: {
          baseURL: mockBaseURL,
          apiKey: 'mock-key',
        },
      },
      supportedClientTypes: ['claude'],
      supportModels: ['*'],
    },
    jwt,
  );
  assert(provider.id, 'Provider should have an ID');
  console.log(`✅ Provider created: id=${provider.id}`);

  // --- Setup: Create 3 projects ---
  console.log('\n--- Setup: Create Projects ---');
  const ts = Date.now();
  const projectNames = [`Proj-A-${ts}`, `Proj-B-${ts}`, `Proj-C-${ts}`];
  const projects = [];
  for (let i = 0; i < 3; i++) {
    const p = await adminAPI(
      'POST',
      '/projects',
      {
        name: projectNames[i],
        slug: `proj-${String.fromCharCode(97 + i)}-${ts}`,
        enabledCustomRoutes: ['claude'],
      },
      jwt,
    );
    assert(p.id, `Project ${i} should have an ID`);
    projects.push(p);
    console.log(`✅ Project ${projectNames[i]} created: id=${p.id}`);
  }

  // --- Setup: Create Global Route ---
  await adminAPI(
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
  console.log('✅ Global route created');

  // --- Setup: Create API Tokens and send requests ---
  // Only send requests for projects A and B; project C stays empty (no stats data)
  // This is crucial: switching to empty project C causes chart to unmount ("No data"),
  // switching back to A/B causes chart to remount — triggering the ResponsiveContainer race.
  console.log('\n--- Setup: Create Tokens & Send Requests ---');
  for (let i = 0; i < 3; i++) {
    const tokenResult = await adminAPI(
      'POST',
      '/api-tokens',
      {
        name: `Token-${projectNames[i]}`,
        projectID: projects[i].id,
      },
      jwt,
    );
    assert(tokenResult.token, `Token ${i} should have a value`);

    if (i < 2) {
      // Only projects A and B get requests; C stays empty
      for (let j = 0; j < 5; j++) {
        await sendClaudeRequest(tokenResult.token);
      }
      console.log(`✅ Sent 5 requests via ${projectNames[i]} token`);
    } else {
      console.log(`⏭️  Skipped requests for ${projectNames[i]} (empty project for testing)`);
    }
  }

  // Wait for stats aggregation
  await new Promise((r) => setTimeout(r, 2000));

  // --- Browser Test ---
  console.log('\n--- Browser: Launch ---');
  browser = await chromium.launch({ headless: !HEADED });
  const context = await browser.newContext();
  const page = await context.newPage();

  // Collect console errors AND warnings (Recharts uses console.warn for dimension issues)
  const consoleErrors = [];
  const consoleWarnings = [];
  page.on('console', (msg) => {
    if (msg.type() === 'error') {
      consoleErrors.push(msg.text());
    }
    if (msg.type() === 'warning') {
      consoleWarnings.push(msg.text());
    }
  });

  // Also collect page errors (uncaught exceptions)
  const pageErrors = [];
  page.on('pageerror', (err) => {
    pageErrors.push(err.message);
  });

  // Step 1: Login
  console.log('\n--- Step 1: Browser Login ---');
  await page.goto(BASE);
  await page.waitForSelector('input[type="text"]', { timeout: 10000 });
  await page.fill('input[type="text"]', USER);
  await page.fill('input[type="password"]', PASS);
  await page.locator('button[type="submit"]').click();
  await page.waitForTimeout(2000);
  console.log('✅ Browser login success');

  // Step 2: Navigate to Stats page
  console.log('\n--- Step 2: Navigate to Stats ---');
  await page.goto(`${BASE}/stats`);
  await page.waitForTimeout(3000);

  const statsBody = await page.textContent('body');
  assert(
    statsBody.includes('Stats') ||
      statsBody.includes('stats') ||
      statsBody.includes('统计') ||
      statsBody.includes('Statistics'),
    'Should show Stats page',
  );
  console.log('✅ Stats page loaded');

  // Step 3: Verify chart is visible
  console.log('\n--- Step 3: Verify Chart Renders ---');
  const chartContainer = page.locator('.recharts-wrapper').first();
  await chartContainer.waitFor({ state: 'visible', timeout: 15000 });
  assert((await chartContainer.count()) > 0, 'Chart should be visible on stats page');
  console.log('✅ Chart visible');

  // Step 4: Force chart container to 0 dimensions to trigger ResponsiveContainer bug.
  // This simulates the real-world scenario where the container briefly has 0 dimensions
  // during layout reflows (e.g., sidebar toggle, window resize crossing breakpoints,
  // or browser tab becoming hidden/visible).
  console.log('\n--- Step 4: Trigger ResponsiveContainer Dimension Bug ---');
  await page.waitForTimeout(1000);

  for (let i = 0; i < 30; i++) {
    await page.evaluate(() => {
      const containers = document.querySelectorAll('.recharts-wrapper');
      containers.forEach((c) => {
        const parent = c.parentElement;
        if (parent) {
          parent.style.width = '0px';
          parent.style.height = '0px';
          parent.style.overflow = 'hidden';
        }
      });
    });
    await page.waitForTimeout(20);

    await page.evaluate(() => {
      const containers = document.querySelectorAll('.recharts-wrapper');
      containers.forEach((c) => {
        const parent = c.parentElement;
        if (parent) {
          parent.style.width = '';
          parent.style.height = '';
          parent.style.overflow = '';
        }
      });
    });
    await page.waitForTimeout(30);

    // Also switch projects during some iterations
    if (i % 5 === 0) {
      const chip = page.getByRole('button', { name: projectNames[i % 3], exact: true });
      if ((await chip.count()) > 0) {
        await chip.click();
      } else {
        console.log(`  ⚠️ Project chip "${projectNames[i % 3]}" not found, skipping`);
      }
      await page.waitForTimeout(50);
    }
  }

  // Verify chart recovers after resize cycle
  console.log('\n--- Step 4b: Verify Chart Recovery ---');
  const recoveredChart = page.locator('.recharts-wrapper').first();
  await recoveredChart.waitFor({ state: 'visible', timeout: 10000 });
  assert((await recoveredChart.count()) > 0, 'Chart should be visible after resize cycle');
  console.log('✅ Chart recovered after resize cycle');

  // Wait for deferred errors
  await page.waitForTimeout(3000);

  // Step 5: Check for the specific Recharts error/warning
  console.log('\n--- Step 5: Check Console Errors & Warnings ---');
  const allMessages = [...consoleErrors, ...consoleWarnings, ...pageErrors];
  const rechartsMessages = allMessages.filter(
    (e) => e.includes('width') && e.includes('height') && e.includes('greater than 0'),
  );

  console.log(`  Total console errors: ${consoleErrors.length}`);
  console.log(`  Total console warnings: ${consoleWarnings.length}`);
  console.log(`  Total page errors: ${pageErrors.length}`);
  console.log(`  Recharts dimension messages: ${rechartsMessages.length}`);

  if (consoleErrors.length > 0) {
    console.log('\n  Console errors:');
    for (const e of consoleErrors) {
      console.log(`    - ${e.substring(0, 200)}`);
    }
  }
  if (consoleWarnings.length > 0) {
    console.log('\n  Console warnings:');
    for (const e of consoleWarnings) {
      console.log(`    - ${e.substring(0, 200)}`);
    }
  }
  if (pageErrors.length > 0) {
    console.log('\n  Page errors:');
    for (const e of pageErrors) {
      console.log(`    - ${e.substring(0, 200)}`);
    }
  }

  const hasRechartsError = rechartsMessages.length > 0;
  if (hasRechartsError) {
    console.log('\n🐛 BUG: Recharts width/height warning detected!');
    console.log('   Issue #220 regression.');
    exitCode = 1;
  } else {
    console.log('\n✅ No Recharts dimension warnings — fix is working.');
  }

  // Take screenshot of final state
  const screenshotPath = path.join(os.tmpdir(), 'stats-chart-resize-result.png');
  await page.screenshot({ path: screenshotPath });
  console.log(`  Screenshot: ${screenshotPath}`);

  console.log(`\n===== Test ${exitCode === 0 ? 'PASSED' : 'FAILED'} =====`);
  await browser.close();
  mockServer.close();
  process.exit(exitCode);
})().catch(async (err) => {
  console.error('❌ Test error:', err.message);
  if (browser) {
    try { await browser.close(); } catch {}
  }
  if (mockServer) mockServer.close();
  process.exit(1);
});
