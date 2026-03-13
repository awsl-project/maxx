/**
 * Playwright E2E Test: Codex Provider - reasoning & service_tier overrides
 *
 * 使用方式：
 *   npx playwright test -c playwright.config.ts codex-reasoning-servicetier.spec.ts --project=e2e-chromium
 */
import http from 'node:http';

import { expect, test } from 'playwright/test';

import { BASE, PASS, USER, adminAPI } from './helpers';

test.describe.configure({ mode: 'serial' });

type CapturedRequest = { url: string | undefined; body: any };

function startMockCodexServer(): Promise<{ server: http.Server; port: number; captured: CapturedRequest[] }> {
  const captured: CapturedRequest[] = [];

  return new Promise((resolve) => {
    const server = http.createServer((req, res) => {
      let raw = '';
      req.on('data', (chunk) => {
        raw += chunk;
      });
      req.on('end', () => {
        let parsed: any = {};
        try {
          parsed = JSON.parse(raw);
        } catch {
          // ignore malformed JSON in the mock
        }

        captured.push({ url: req.url, body: parsed });

        const model = parsed.model || 'o3-mini';
        const responseId = `resp_mock_${Date.now()}`;
        const now = Math.floor(Date.now() / 1000);

        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(
          JSON.stringify({
            id: responseId,
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
      const address = server.address();
      if (!address || typeof address === 'string') {
        throw new Error('Failed to determine mock server port');
      }
      console.log(`✅ Mock Codex API server started on port ${address.port}`);
      resolve({ server, port: address.port, captured });
    });
  });
}

async function sendCodexRequest(apiToken: string, body: unknown) {
  const response = await fetch(`${BASE}/v1/responses`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${apiToken}`,
    },
    body: JSON.stringify(body),
  });

  const text = await response.text();
  if (!response.ok) {
    throw new Error(`Proxy request failed (${response.status}): ${text}`);
  }
  return JSON.parse(text);
}

test('codex provider overrides reasoning and service tier as configured', async () => {
  const mock = await startMockCodexServer();
  let jwt: string | null = null;
  let apiTokenId: number | null = null;
  const routeIds: number[] = [];
  const providerIds: number[] = [];

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

    const tokenResult = await adminAPI(
      'POST',
      '/api-tokens',
      { name: 'Codex Test Token 1', description: 'For reasoning/serviceTier test' },
      jwt,
    );
    expect(tokenResult.token).toBeTruthy();
    const apiToken = tokenResult.token as string;
    apiTokenId = tokenResult.apiToken.id as number;
    console.log('✅ API token created');

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
            baseURL: `http://127.0.0.1:${mock.port}`,
            reasoning: 'high',
            serviceTier: 'priority',
          },
        },
        supportedClientTypes: ['codex'],
        supportModels: ['*'],
      },
      jwt,
    );
    providerIds.push(provider1.id);
    const route1 = await adminAPI(
      'POST',
      '/routes',
      { isEnabled: true, isNative: false, clientType: 'codex', providerID: provider1.id, projectID: 0, position: 1 },
      jwt,
    );
    routeIds.push(route1.id);

    mock.captured.length = 0;
    expect(
      (await sendCodexRequest(apiToken, {
        model: 'o3-mini',
        input: 'Hello, test!',
        reasoning: { effort: 'low', summary: 'auto' },
        service_tier: 'flex',
        max_output_tokens: 100,
      })).id,
    ).toBeTruthy();
    expect(mock.captured.at(-1)?.body.reasoning?.effort).toBe('high');
    expect(mock.captured.at(-1)?.body.service_tier).toBe('priority');
    console.log('✅ Test 1 PASSED: reasoning=high, serviceTier=priority correctly overridden');

    await adminAPI('PUT', `/routes/${route1.id}`, { ...route1, isEnabled: false }, jwt);

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
            baseURL: `http://127.0.0.1:${mock.port}`,
            reasoning: 'low',
            serviceTier: 'flex',
          },
        },
        supportedClientTypes: ['codex'],
        supportModels: ['*'],
      },
      jwt,
    );
    providerIds.push(provider2.id);
    const route2 = await adminAPI(
      'POST',
      '/routes',
      { isEnabled: true, isNative: false, clientType: 'codex', providerID: provider2.id, projectID: 0, position: 1 },
      jwt,
    );
    routeIds.push(route2.id);

    mock.captured.length = 0;
    expect(
      (await sendCodexRequest(apiToken, {
        model: 'o3-mini',
        input: 'Hello again!',
        reasoning: { effort: 'high', summary: 'auto' },
        service_tier: 'priority',
        max_output_tokens: 200,
      })).id,
    ).toBeTruthy();
    expect(mock.captured.at(-1)?.body.reasoning?.effort).toBe('low');
    expect(mock.captured.at(-1)?.body.service_tier).toBe('flex');
    console.log('✅ Test 2 PASSED: reasoning=low, serviceTier=flex correctly overridden');

    await adminAPI('PUT', `/routes/${route2.id}`, { ...route2, isEnabled: false }, jwt);

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
            baseURL: `http://127.0.0.1:${mock.port}`,
          },
        },
        supportedClientTypes: ['codex'],
        supportModels: ['*'],
      },
      jwt,
    );
    providerIds.push(provider3.id);
    const route3 = await adminAPI(
      'POST',
      '/routes',
      { isEnabled: true, isNative: false, clientType: 'codex', providerID: provider3.id, projectID: 0, position: 1 },
      jwt,
    );
    routeIds.push(route3.id);

    mock.captured.length = 0;
    expect(
      (await sendCodexRequest(apiToken, {
        model: 'o3-mini',
        input: 'Pass-through test',
        reasoning: { effort: 'medium', summary: 'auto' },
        service_tier: 'auto',
        max_output_tokens: 50,
      })).id,
    ).toBeTruthy();
    expect(mock.captured.at(-1)?.body.reasoning?.effort).toBe('medium');
    expect(mock.captured.at(-1)?.body.service_tier).toBe('auto');
    console.log('✅ Test 3 PASSED: No override, values passed through correctly');

    await adminAPI('PUT', `/routes/${route3.id}`, { ...route3, isEnabled: false }, jwt);

    console.log('\n========== Test 4: Dynamic update — add overrides ==========');
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

    await new Promise((resolve) => setTimeout(resolve, 500));

    mock.captured.length = 0;
    expect(
      (await sendCodexRequest(apiToken, {
        model: 'o3-mini',
        input: 'Dynamic update test',
        reasoning: { effort: 'low' },
        max_output_tokens: 50,
      })).id,
    ).toBeTruthy();
    expect(mock.captured.at(-1)?.body.reasoning?.effort).toBe('high');
    expect(mock.captured.at(-1)?.body.service_tier).toBe('priority');
    console.log('✅ Test 4 PASSED: Dynamic update applied correctly');

    await adminAPI('PUT', `/routes/${route3.id}`, { ...route3, isEnabled: false }, jwt);

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
            baseURL: `http://127.0.0.1:${mock.port}`,
            reasoning: 'high',
            serviceTier: 'flex',
          },
        },
        supportedClientTypes: ['codex'],
        supportModels: ['*'],
      },
      jwt,
    );
    providerIds.push(provider5.id);
    const route5 = await adminAPI(
      'POST',
      '/routes',
      { isEnabled: true, isNative: false, clientType: 'codex', providerID: provider5.id, projectID: 0, position: 1 },
      jwt,
    );
    routeIds.push(route5.id);

    mock.captured.length = 0;
    expect(
      (await sendCodexRequest(apiToken, {
        model: 'o3-mini',
        input: 'No reasoning or service_tier from client',
        max_output_tokens: 50,
      })).id,
    ).toBeTruthy();
    expect(mock.captured.at(-1)?.body.reasoning?.effort).toBe('high');
    expect(mock.captured.at(-1)?.body.service_tier).toBe('flex');
    console.log('✅ Test 5 PASSED: Provider overrides injected when client omits fields');

    console.log('✅ Cleanup completed');
  } finally {
    if (apiTokenId) {
      try {
        await adminAPI('DELETE', `/api-tokens/${apiTokenId}`, undefined, jwt ?? undefined);
      } catch {}
    }
    for (const id of routeIds.reverse()) {
      try {
        await adminAPI('DELETE', `/routes/${id}`, undefined, jwt ?? undefined);
      } catch {}
    }
    for (const id of providerIds.reverse()) {
      try {
        await adminAPI('DELETE', `/providers/${id}`, undefined, jwt ?? undefined);
      } catch {}
    }
    await new Promise((resolve) => mock.server.close(() => resolve(undefined)));
  }
});
