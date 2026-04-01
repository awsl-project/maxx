import { execSync, spawn } from 'child_process';
import { resolve } from 'path';
import { fileURLToPath } from 'url';

const MAXX_PORT = 19881;
const MOCK_PORT = 19999;
const MAXX_URL = `http://localhost:${MAXX_PORT}`;
const MOCK_URL = `http://localhost:${MOCK_PORT}`;

async function waitForServer(url: string, timeoutMs = 30000): Promise<void> {
  const start = Date.now();
  while (Date.now() - start < timeoutMs) {
    try {
      const resp = await fetch(url);
      if (resp.ok) return;
    } catch {}
    await new Promise((r) => setTimeout(r, 300));
  }
  throw new Error(`Server at ${url} not ready after ${timeoutMs}ms`);
}

async function createProvider(name: string, types: string[], apiKey: string) {
  const resp = await fetch(`${MAXX_URL}/api/admin/providers`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      type: 'custom',
      name,
      supportedClientTypes: types,
      config: { custom: { baseURL: MOCK_URL, apiKey } },
    }),
  });
  const data = await resp.json();
  return data.id as number;
}

async function createRoute(providerID: number, clientType: string) {
  await fetch(`${MAXX_URL}/api/admin/routes`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      providerID,
      clientType,
      isEnabled: true,
      isNative: true,
      position: 1,
      weight: 1,
    }),
  });
}

export default async function globalSetup() {
  // Resolve project root (web/e2e/global-setup.ts → ../../)
  const thisFile = fileURLToPath(import.meta.url);
  const projectRoot = resolve(thisFile, '..', '..', '..');

  console.log('[Setup] Building mock server...');
  execSync('go build -o /tmp/maxx-mockserver ./cmd/mockserver', {
    cwd: projectRoot,
    stdio: 'pipe',
  });

  console.log('[Setup] Building maxx...');
  execSync('go build -o /tmp/maxx-test ./cmd/maxx', {
    cwd: projectRoot,
    stdio: 'pipe',
  });

  console.log('[Setup] Starting mock server on :' + MOCK_PORT);
  const mockProcess = spawn('/tmp/maxx-mockserver', ['-addr', `:${MOCK_PORT}`], {
    stdio: 'pipe',
    detached: true,
  });
  mockProcess.unref();

  console.log('[Setup] Starting maxx on :' + MAXX_PORT + ' (in-memory SQLite)');
  const maxxProcess = spawn('/tmp/maxx-test', ['-addr', `:${MAXX_PORT}`], {
    stdio: 'pipe',
    detached: true,
    env: { ...process.env, MAXX_DSN: 'sqlite://:memory:' },
  });
  maxxProcess.unref();

  // Wait for both servers
  console.log('[Setup] Waiting for servers...');
  await waitForServer(`${MOCK_URL}/v1/chat/completions`);
  await waitForServer(`${MAXX_URL}/health`);
  console.log('[Setup] Servers ready');

  // Create test providers with distinct API keys
  const p1 = await createProvider('Gemini Provider', ['gemini', 'openai'], 'key-gemini');
  const p2 = await createProvider('OpenRouter', ['openai', 'claude'], 'key-openrouter');
  const p3 = await createProvider('Claude Direct', ['claude'], 'key-claude');
  const p4 = await createProvider('Azure OpenAI', ['openai'], 'key-azure');

  // Create routes
  await createRoute(p1, 'gemini');
  await createRoute(p1, 'openai');
  await createRoute(p2, 'openai');
  await createRoute(p2, 'claude');
  await createRoute(p3, 'claude');
  await createRoute(p4, 'openai');

  // Store PIDs for teardown
  process.env.MOCK_PID = String(mockProcess.pid);
  process.env.MAXX_PID = String(maxxProcess.pid);

  console.log(`[Setup] Done. Mock PID=${mockProcess.pid}, Maxx PID=${maxxProcess.pid}`);
}
