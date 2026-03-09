/**
 * Playwright E2E Test: Codex Provider - reasoning & service_tier overrides
 *
 * 测试流程：
 * 1. 启动 mock Codex API 服务，记录收到的请求 body
 * 2. 通过 Admin API 创建 codex provider（指向 mock 服务，配置 reasoning / serviceTier）
 * 3. 创建 route、启用 API token 认证、创建 API token
 * 4. 通过 /v1/responses 代理发送 codex 请求
 * 5. 验证 mock 服务收到的请求中 reasoning.effort 和 service_tier 被正确覆盖
 *
 * 使用方式：
 *   先启动 maxx 服务器（需要开启 auth），然后运行：
 *   node test-codex-reasoning-servicetier.mjs [base_url] [username] [password]
 *
 *   默认值：
 *     base_url = http://localhost:9880
 *     username = admin
 *     password = test123
 */
import http from 'node:http';

const BASE = process.argv[2] || 'http://localhost:9880';
const USER = process.argv[3] || 'admin';
const PASS = process.argv[4] || 'test123';

let exitCode = 0;
let mockServer = null;

function assert(condition, msg) {
  if (!condition) {
    console.error(`❌ ASSERTION FAILED: ${msg}`);
    exitCode = 1;
    throw new Error(msg);
  }
}

// ===== Mock Codex API Server =====
// Records every request body and returns a valid Codex Responses API response
function startMockCodexServer() {
  /** @type {Array<{url: string, body: object}>} */
  const captured = [];

  return new Promise((resolve) => {
    const server = http.createServer((req, res) => {
      let raw = '';
      req.on('data', (chunk) => (raw += chunk));
      req.on('end', () => {
        let parsed = {};
        try {
          parsed = JSON.parse(raw);
        } catch {}

        captured.push({ url: req.url, body: parsed });

        const model = parsed.model || 'o3-mini';
        const respId = `resp_mock_${Date.now()}`;
        const now = Math.floor(Date.now() / 1000);

        // Non-stream: return compact Codex Responses format
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(
          JSON.stringify({
            id: respId,
            object: 'response',
            created_at: now,
            model,
            status: 'completed',
            output: [
              {
                type: 'message',
                id: `msg_mock_${Date.now()}`,
                role: 'assistant',
                status: 'completed',
                content: [{ type: 'output_text', text: 'Hello from mock Codex!' }],
              },
            ],
            usage: {
              input_tokens: 20,
              output_tokens: 10,
              total_tokens: 30,
            },
          }),
        );
      });
    });

    server.listen(0, '127.0.0.1', () => {
      const port = server.address().port;
      console.log(`✅ Mock Codex API server started on port ${port}`);
      resolve({ server, port, captured });
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

// ===== Send Codex (Responses API) request through proxy =====
async function sendCodexRequest(apiToken, body) {
  const res = await fetch(`${BASE}/v1/responses`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${apiToken}`,
    },
    body: JSON.stringify(body),
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
  console.log('\n--- Setup: Mock Codex API Server ---');
  const mock = await startMockCodexServer();
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

  // =====================================================================
  // Test 1: Provider with reasoning=high, serviceTier=priority
  // =====================================================================
  console.log('\n========== Test 1: reasoning=high, serviceTier=priority ==========');

  const provider1 = await adminAPI(
    'POST',
    '/providers',
    {
      name: `Codex-Override-Test-${Date.now()}`,
      type: 'codex',
      config: {
        codex: {
          email: 'test@example.com',
          refreshToken: 'fake-token',
          accessToken: 'mock-access-token',
          baseURL: mockBaseURL,
          reasoning: 'high',
          serviceTier: 'priority',
        },
      },
      supportedClientTypes: ['codex'],
      supportModels: ['*'],
    },
    jwt,
  );
  assert(provider1.id, 'Provider 1 should have an ID');
  console.log(`✅ Provider created: id=${provider1.id}`);

  // Create route
  const route1 = await adminAPI(
    'POST',
    '/routes',
    {
      isEnabled: true,
      isNative: false,
      clientType: 'codex',
      providerID: provider1.id,
      projectID: 0,
      position: 1,
    },
    jwt,
  );
  console.log(`✅ Route created: id=${route1.id}`);

  // Create API token
  const token1Result = await adminAPI(
    'POST',
    '/api-tokens',
    { name: 'Codex Test Token 1', description: 'For reasoning/serviceTier test' },
    jwt,
  );
  assert(token1Result.token, 'Token 1 should have a value');
  console.log(`✅ API token created`);

  // Send request with low reasoning and no service_tier — provider should override both
  mock.captured.length = 0; // clear
  const resp1 = await sendCodexRequest(token1Result.token, {
    model: 'o3-mini',
    input: 'Hello, test!',
    reasoning: { effort: 'low', summary: 'auto' },
    service_tier: 'flex',
    max_output_tokens: 100,
  });
  assert(resp1.id, 'Response 1 should have an ID');
  console.log('✅ Proxy request succeeded');

  // Verify captured request
  assert(mock.captured.length >= 1, `Should have captured at least 1 request, got ${mock.captured.length}`);
  const captured1 = mock.captured[mock.captured.length - 1].body;
  console.log(`  Captured reasoning.effort: ${captured1.reasoning?.effort}`);
  console.log(`  Captured service_tier: ${captured1.service_tier}`);

  assert(
    captured1.reasoning?.effort === 'high',
    `reasoning.effort should be overridden to "high", got "${captured1.reasoning?.effort}"`,
  );
  assert(
    captured1.service_tier === 'priority',
    `service_tier should be overridden to "priority", got "${captured1.service_tier}"`,
  );
  console.log('✅ Test 1 PASSED: reasoning=high, serviceTier=priority correctly overridden');

  // Cleanup: disable route
  await adminAPI('PUT', `/routes/${route1.id}`, { ...route1, isEnabled: false }, jwt);

  // =====================================================================
  // Test 2: Provider with reasoning=low, serviceTier=flex
  // =====================================================================
  console.log('\n========== Test 2: reasoning=low, serviceTier=flex ==========');

  const provider2 = await adminAPI(
    'POST',
    '/providers',
    {
      name: `Codex-Override-Test2-${Date.now()}`,
      type: 'codex',
      config: {
        codex: {
          email: 'test2@example.com',
          refreshToken: 'fake-token-2',
          accessToken: 'mock-access-token-2',
          baseURL: mockBaseURL,
          reasoning: 'low',
          serviceTier: 'flex',
        },
      },
      supportedClientTypes: ['codex'],
      supportModels: ['*'],
    },
    jwt,
  );
  assert(provider2.id, 'Provider 2 should have an ID');

  const route2 = await adminAPI(
    'POST',
    '/routes',
    {
      isEnabled: true,
      isNative: false,
      clientType: 'codex',
      providerID: provider2.id,
      projectID: 0,
      position: 1,
    },
    jwt,
  );

  // Send request with high reasoning and priority tier — should be overridden to low/flex
  mock.captured.length = 0;
  const resp2 = await sendCodexRequest(token1Result.token, {
    model: 'o3-mini',
    input: 'Hello again!',
    reasoning: { effort: 'high', summary: 'auto' },
    service_tier: 'priority',
    max_output_tokens: 200,
  });
  assert(resp2.id, 'Response 2 should have an ID');

  const captured2 = mock.captured[mock.captured.length - 1].body;
  console.log(`  Captured reasoning.effort: ${captured2.reasoning?.effort}`);
  console.log(`  Captured service_tier: ${captured2.service_tier}`);

  assert(
    captured2.reasoning?.effort === 'low',
    `reasoning.effort should be overridden to "low", got "${captured2.reasoning?.effort}"`,
  );
  assert(
    captured2.service_tier === 'flex',
    `service_tier should be overridden to "flex", got "${captured2.service_tier}"`,
  );
  console.log('✅ Test 2 PASSED: reasoning=low, serviceTier=flex correctly overridden');

  await adminAPI('PUT', `/routes/${route2.id}`, { ...route2, isEnabled: false }, jwt);

  // =====================================================================
  // Test 3: Provider with NO overrides — should pass through client values
  // =====================================================================
  console.log('\n========== Test 3: No overrides (pass-through) ==========');

  const provider3 = await adminAPI(
    'POST',
    '/providers',
    {
      name: `Codex-NoOverride-Test-${Date.now()}`,
      type: 'codex',
      config: {
        codex: {
          email: 'test3@example.com',
          refreshToken: 'fake-token-3',
          accessToken: 'mock-access-token-3',
          baseURL: mockBaseURL,
          // No reasoning or serviceTier override
        },
      },
      supportedClientTypes: ['codex'],
      supportModels: ['*'],
    },
    jwt,
  );
  assert(provider3.id, 'Provider 3 should have an ID');

  const route3 = await adminAPI(
    'POST',
    '/routes',
    {
      isEnabled: true,
      isNative: false,
      clientType: 'codex',
      providerID: provider3.id,
      projectID: 0,
      position: 1,
    },
    jwt,
  );

  // Send request with specific values — should pass through unchanged
  mock.captured.length = 0;
  const resp3 = await sendCodexRequest(token1Result.token, {
    model: 'o3-mini',
    input: 'Pass-through test',
    reasoning: { effort: 'medium', summary: 'auto' },
    service_tier: 'auto',
    max_output_tokens: 50,
  });
  assert(resp3.id, 'Response 3 should have an ID');

  const captured3 = mock.captured[mock.captured.length - 1].body;
  console.log(`  Captured reasoning.effort: ${captured3.reasoning?.effort}`);
  console.log(`  Captured service_tier: ${captured3.service_tier}`);

  assert(
    captured3.reasoning?.effort === 'medium',
    `reasoning.effort should pass through as "medium", got "${captured3.reasoning?.effort}"`,
  );
  assert(
    captured3.service_tier === 'auto',
    `service_tier should pass through as "auto", got "${captured3.service_tier}"`,
  );
  console.log('✅ Test 3 PASSED: No override, values passed through correctly');

  await adminAPI('PUT', `/routes/${route3.id}`, { ...route3, isEnabled: false }, jwt);

  // =====================================================================
  // Test 4: Update provider to add overrides dynamically
  // =====================================================================
  console.log('\n========== Test 4: Dynamic update — add overrides ==========');

  // Re-enable route3 and update provider3 to add overrides
  await adminAPI('PUT', `/routes/${route3.id}`, { ...route3, isEnabled: true }, jwt);
  await adminAPI(
    'PUT',
    `/providers/${provider3.id}`,
    {
      ...provider3,
      config: {
        codex: {
          ...provider3.config.codex,
          reasoning: 'high',
          serviceTier: 'priority',
        },
      },
    },
    jwt,
  );
  console.log('✅ Provider updated with reasoning=high, serviceTier=priority');

  // Wait for provider cache to refresh
  await new Promise((r) => setTimeout(r, 500));

  mock.captured.length = 0;
  const resp4 = await sendCodexRequest(token1Result.token, {
    model: 'o3-mini',
    input: 'Dynamic update test',
    reasoning: { effort: 'low' },
    max_output_tokens: 50,
  });
  assert(resp4.id, 'Response 4 should have an ID');

  const captured4 = mock.captured[mock.captured.length - 1].body;
  console.log(`  Captured reasoning.effort: ${captured4.reasoning?.effort}`);
  console.log(`  Captured service_tier: ${captured4.service_tier}`);

  assert(
    captured4.reasoning?.effort === 'high',
    `After update, reasoning.effort should be "high", got "${captured4.reasoning?.effort}"`,
  );
  assert(
    captured4.service_tier === 'priority',
    `After update, service_tier should be "priority", got "${captured4.service_tier}"`,
  );
  console.log('✅ Test 4 PASSED: Dynamic update applied correctly');

  await adminAPI('PUT', `/routes/${route3.id}`, { ...route3, isEnabled: false }, jwt);

  // =====================================================================
  // Test 5: Client sends NO reasoning / service_tier, provider has overrides
  // =====================================================================
  console.log('\n========== Test 5: Client omits fields, provider overrides ==========');

  const provider5 = await adminAPI(
    'POST',
    '/providers',
    {
      name: `Codex-ClientOmit-Test-${Date.now()}`,
      type: 'codex',
      config: {
        codex: {
          email: 'test5@example.com',
          refreshToken: 'fake-token-5',
          accessToken: 'mock-access-token-5',
          baseURL: mockBaseURL,
          reasoning: 'high',
          serviceTier: 'flex',
        },
      },
      supportedClientTypes: ['codex'],
      supportModels: ['*'],
    },
    jwt,
  );
  assert(provider5.id, 'Provider 5 should have an ID');

  const route5 = await adminAPI(
    'POST',
    '/routes',
    {
      isEnabled: true,
      isNative: false,
      clientType: 'codex',
      providerID: provider5.id,
      projectID: 0,
      position: 1,
    },
    jwt,
  );

  // Send request WITHOUT reasoning and service_tier
  mock.captured.length = 0;
  const resp5 = await sendCodexRequest(token1Result.token, {
    model: 'o3-mini',
    input: 'No reasoning or service_tier from client',
    max_output_tokens: 50,
  });
  assert(resp5.id, 'Response 5 should have an ID');

  const captured5 = mock.captured[mock.captured.length - 1].body;
  console.log(`  Captured reasoning.effort: ${captured5.reasoning?.effort}`);
  console.log(`  Captured service_tier: ${captured5.service_tier}`);

  assert(
    captured5.reasoning?.effort === 'high',
    `reasoning.effort should be injected as "high" even when client omits it, got "${captured5.reasoning?.effort}"`,
  );
  assert(
    captured5.service_tier === 'flex',
    `service_tier should be injected as "flex" even when client omits it, got "${captured5.service_tier}"`,
  );
  console.log('✅ Test 5 PASSED: Provider overrides injected when client omits fields');

  await adminAPI('PUT', `/routes/${route5.id}`, { ...route5, isEnabled: false }, jwt);

  // ===== Cleanup =====
  console.log('\n--- Cleanup: Deleting test providers and routes ---');
  const routeIds = [route1.id, route2.id, route3.id, route5.id];
  const providerIds = [provider1.id, provider2.id, provider3.id, provider5.id];
  for (const id of routeIds) {
    try { await adminAPI('DELETE', `/routes/${id}`, null, jwt); } catch {}
  }
  for (const id of providerIds) {
    try { await adminAPI('DELETE', `/providers/${id}`, null, jwt); } catch {}
  }
  console.log('✅ Cleanup completed');

  // ===== Summary =====
  console.log(`\n===== All Tests ${exitCode === 0 ? 'PASSED' : 'FAILED'} =====`);
  mockServer.close();
  process.exit(exitCode);
})().catch(async (err) => {
  console.error('❌ Test error:', err.message);
  if (mockServer) mockServer.close();
  process.exit(1);
});
