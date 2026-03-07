/**
 * Playwright E2E Test: Requests Page - Project Filter
 *
 * 测试流程：
 * 1. 启动 mock Claude API 服务
 * 2. 通过 Admin API 创建 provider (指向 mock 服务)、project、route、API token
 * 3. 启用 API token 认证
 * 4. 通过代理发送请求，使请求记录出现在 Requests 页面
 * 5. 浏览器登录，验证 Project 过滤器的功能
 *
 * 使用方式：
 *   先启动 maxx 服务器（需要开启 auth），然后运行：
 *   node test-requests-project-filter.mjs [base_url] [username] [password]
 *
 *   默认值：
 *     base_url = http://localhost:9880
 *     username = admin
 *     password = test123
 */
import http from 'node:http';
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
          } catch {}

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
  console.log('\n--- Setup: Enable API Token Auth ---');
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
  console.log(`✅ Provider created: id=${provider.id}, name=${provider.name}`);

  // --- Setup: Create two projects ---
  console.log('\n--- Setup: Create Projects ---');
  const ts = Date.now();
  const projectAName = `Alpha-${ts}`;
  const projectBName = `Beta-${ts}`;
  const projectA = await adminAPI(
    'POST',
    '/projects',
    {
      name: projectAName,
      slug: `alpha-${ts}`,
      enabledCustomRoutes: ['claude'],
    },
    jwt,
  );
  assert(projectA.id, 'Project A should have an ID');
  console.log(`✅ Project A created: id=${projectA.id}, name=${projectA.name}`);

  const projectB = await adminAPI(
    'POST',
    '/projects',
    {
      name: projectBName,
      slug: `beta-${ts}`,
      enabledCustomRoutes: ['claude'],
    },
    jwt,
  );
  assert(projectB.id, 'Project B should have an ID');
  console.log(`✅ Project B created: id=${projectB.id}, name=${projectB.name}`);

  // --- Setup: Create Route ---
  console.log('\n--- Setup: Create Routes ---');
  const globalRoute = await adminAPI(
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
  console.log(`✅ Global route created: id=${globalRoute.id}`);

  // --- Setup: Create API Tokens (one per project) ---
  console.log('\n--- Setup: Create API Tokens ---');
  const tokenAResult = await adminAPI(
    'POST',
    '/api-tokens',
    {
      name: 'Token for Alpha',
      description: 'Test token for Project Alpha',
      projectID: projectA.id,
    },
    jwt,
  );
  assert(tokenAResult.token, 'Token A should have a token value');
  const tokenA = tokenAResult.token;
  console.log(`✅ Token A created: prefix=${tokenAResult.apiToken.tokenPrefix}`);

  const tokenBResult = await adminAPI(
    'POST',
    '/api-tokens',
    {
      name: 'Token for Beta',
      description: 'Test token for Project Beta',
      projectID: projectB.id,
    },
    jwt,
  );
  assert(tokenBResult.token, 'Token B should have a token value');
  const tokenB = tokenBResult.token;
  console.log(`✅ Token B created: prefix=${tokenBResult.apiToken.tokenPrefix}`);

  // --- Setup: Send proxy requests ---
  console.log('\n--- Setup: Send Proxy Requests ---');
  for (let i = 0; i < 3; i++) {
    const resp = await sendClaudeRequest(tokenA);
    assert(resp.id, `Alpha request ${i + 1} should succeed`);
  }
  console.log('✅ Sent 3 requests via Project Alpha token');

  for (let i = 0; i < 2; i++) {
    const resp = await sendClaudeRequest(tokenB);
    assert(resp.id, `Beta request ${i + 1} should succeed`);
  }
  console.log('✅ Sent 2 requests via Project Beta token');

  // Wait for requests to be processed
  await new Promise((r) => setTimeout(r, 1000));

  // --- Browser Test ---
  console.log('\n--- Browser: Launch ---');
  browser = await chromium.launch({ headless: !HEADED });
  const context = await browser.newContext();
  const page = await context.newPage();

  // Step 1: Login
  console.log('\n--- Step 1: Browser Login ---');
  await page.goto(BASE);
  await page.waitForSelector('input[type="text"]', { timeout: 10000 });
  await page.fill('input[type="text"]', USER);
  await page.fill('input[type="password"]', PASS);
  await page.locator('button[type="submit"]').click();
  await page.waitForTimeout(2000);
  const bodyText = await page.textContent('body');
  assert(
    bodyText.includes('Dashboard') || bodyText.includes('dashboard'),
    'Should reach Dashboard after login',
  );
  console.log('✅ Browser login success');

  // Step 2: Navigate to Requests page
  console.log('\n--- Step 2: Navigate to Requests ---');
  await page.goto(`${BASE}/requests`);
  await page.waitForTimeout(3000);
  const requestsBody = await page.textContent('body');
  assert(requestsBody.includes('total requests'), 'Should show total requests count');
  console.log('✅ Requests page loaded');

  // Step 3: Verify filter mode selector has Project option
  console.log('\n--- Step 3: Verify Project Filter Option ---');
  const filterModeSelect = page.locator('main [role="combobox"]').first();

  // Poll until Project option appears (projects query may still be loading)
  let hasProjectOption = false;
  for (let attempt = 0; attempt < 10; attempt++) {
    await filterModeSelect.click();
    await page.waitForTimeout(500);

    const opt = page.locator('[role="option"]').filter({ hasText: /Project|项目/ });
    hasProjectOption = (await opt.count()) > 0;
    if (hasProjectOption) break;

    await page.keyboard.press('Escape');
    await page.waitForTimeout(1000);
  }
  assert(hasProjectOption, 'Filter mode should have Project option since projects exist');
  console.log('✅ Project option exists in filter mode selector');

  // Step 4: Select Project filter mode
  console.log('\n--- Step 4: Select Project Filter Mode ---');
  const projectOption = page.locator('[role="option"]').filter({ hasText: /Project|项目/ });
  await projectOption.click();
  await page.waitForTimeout(500);

  const filterModeValue = await filterModeSelect.textContent();
  assert(
    filterModeValue.includes('Project') || filterModeValue.includes('项目'),
    `Filter mode should show Project, got: ${filterModeValue}`,
  );
  console.log('✅ Filter mode switched to Project');

  // Step 5: Select Project Alpha and verify count
  console.log('\n--- Step 5: Select Project Alpha ---');
  const projectFilterSelect = page.locator('main [role="combobox"]').nth(1);
  await projectFilterSelect.click();
  await page.waitForTimeout(500);

  const allProjectsOption = page.locator('[role="option"]').filter({ hasText: /All Projects|全部项目/ });
  const alphaOption = page.locator('[role="option"]').filter({ hasText: projectAName });
  const betaOption = page.locator('[role="option"]').filter({ hasText: projectBName });

  assert((await allProjectsOption.count()) > 0, 'Should have All Projects option');
  assert((await alphaOption.count()) > 0, 'Should have Project Alpha option');
  assert((await betaOption.count()) > 0, 'Should have Project Beta option');
  console.log('✅ Project dropdown shows all projects');

  await alphaOption.click();
  await page.waitForTimeout(1500);

  const alphaBody = await page.textContent('body');
  assert(
    alphaBody.includes('3 total'),
    `Should show 3 requests for Alpha, got: ${alphaBody.match(/\d+ total/)?.[0] || 'no match'}`,
  );
  console.log('✅ Project Alpha filter: 3 requests shown');

  // Step 6: Switch to Project Beta and verify count
  console.log('\n--- Step 6: Switch to Project Beta ---');
  await projectFilterSelect.click();
  await page.waitForTimeout(500);
  await page.locator('[role="option"]').filter({ hasText: projectBName }).click();
  await page.waitForTimeout(1500);

  const betaBody = await page.textContent('body');
  assert(
    betaBody.includes('2 total'),
    `Should show 2 requests for Beta, got: ${betaBody.match(/\d+ total/)?.[0] || 'no match'}`,
  );
  console.log('✅ Project Beta filter: 2 requests shown');

  // Step 7: Select "All Projects" to clear filter
  console.log('\n--- Step 7: Clear Project Filter ---');
  await projectFilterSelect.click();
  await page.waitForTimeout(500);
  await page.locator('[role="option"]').filter({ hasText: /All Projects|全部项目/ }).click();
  await page.waitForTimeout(1500);

  const allBody = await page.textContent('body');
  const allMatch = allBody.match(/(\d+) total/);
  assert(allMatch && parseInt(allMatch[1]) >= 5, `Should show >= 5 requests, got: ${allMatch?.[0] || 'no match'}`);
  console.log(`✅ All Projects: ${allMatch[1]} requests shown`);

  // Step 8: Switch back to Token filter mode
  console.log('\n--- Step 8: Switch to Token Filter Mode ---');
  await filterModeSelect.click();
  await page.waitForTimeout(500);
  await page.locator('[role="option"]').filter({ hasText: /^Token$|^令牌$/ }).click();
  await page.waitForTimeout(500);

  const tokenModeValue = await filterModeSelect.textContent();
  assert(
    tokenModeValue.includes('Token') || tokenModeValue.includes('令牌'),
    `Filter mode should show Token, got: ${tokenModeValue}`,
  );
  console.log('✅ Switched back to Token filter mode');

  // Step 9: Test localStorage persistence
  console.log('\n--- Step 9: Test Persistence ---');
  // Switch to project mode and select Alpha
  await filterModeSelect.click();
  await page.waitForTimeout(500);
  await page.locator('[role="option"]').filter({ hasText: /Project|项目/ }).click();
  await page.waitForTimeout(500);

  const projectFilter = page.locator('main [role="combobox"]').nth(1);
  await projectFilter.click();
  await page.waitForTimeout(500);
  await page.locator('[role="option"]').filter({ hasText: projectAName }).click();
  await page.waitForTimeout(1000);

  // Reload
  await page.reload({ waitUntil: 'networkidle' });
  await page.waitForTimeout(3000);

  // Verify persistence
  const modeAfterReload = await page.locator('main [role="combobox"]').first().textContent();
  assert(
    modeAfterReload.includes('Project') || modeAfterReload.includes('项目'),
    `After reload, filter mode should be Project, got: ${modeAfterReload}`,
  );

  const projectAfterReload = await page.locator('main [role="combobox"]').nth(1).textContent();
  assert(
    projectAfterReload.includes(projectAName),
    `After reload, selected project should be ${projectAName}, got: ${projectAfterReload}`,
  );
  console.log('✅ Filter state persisted across page reload');

  // Screenshot
  await page.screenshot({ path: '/tmp/requests-project-filter-result.png' });
  console.log('  Screenshot: /tmp/requests-project-filter-result.png');

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
