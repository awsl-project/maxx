import { execSync, spawn } from 'child_process';
import { resolve, join } from 'path';
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
    } catch {
      // Expected: server not ready yet, will retry
    }
    await new Promise((r) => setTimeout(r, 300));
  }
  throw new Error(`Server at ${url} not ready after ${timeoutMs}ms`);
}

async function createProvider(name: string, types: string[], id?: number) {
  // Use /p/{id} path prefix so mock server can identify which provider is calling
  // ID is predicted (sequential from 1) — we pass it to construct the baseURL
  const baseURL = id ? `${MOCK_URL}/p/${id}` : MOCK_URL;
  const resp = await fetch(`${MAXX_URL}/api/admin/providers`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      type: 'custom',
      name,
      supportedClientTypes: types,
      config: { custom: { baseURL, apiKey: 'mock-key' } },
    }),
  });
  const data = await resp.json();
  return data.id as number;
}

async function createRoute(providerID: number, clientType: string, position = 1) {
  await fetch(`${MAXX_URL}/api/admin/routes`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      providerID,
      clientType,
      isEnabled: true,
      isNative: true,
      position,
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

  console.log('[Setup] Building frontend...');
  execSync('pnpm exec vite build --outDir dist', {
    cwd: join(projectRoot, 'web'),
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

  console.log('[Setup] Starting maxx on :' + MAXX_PORT + ' (in-memory SQLite, mock mode)');
  const maxxProcess = spawn('/tmp/maxx-test', ['-addr', `:${MAXX_PORT}`], {
    stdio: 'pipe',
    detached: true,
    cwd: projectRoot,
    env: { ...process.env, MAXX_DSN: 'sqlite://:memory:', MAXX_MOCK_MODE: '1' },
  });
  maxxProcess.unref();

  // Wait for both servers
  console.log('[Setup] Waiting for servers...');
  await waitForServer(`${MOCK_URL}/v1/chat/completions`);
  await waitForServer(`${MAXX_URL}/health`);
  console.log('[Setup] Servers ready');

  // Create test providers with /p/{id} baseURL prefix for mock server identification
  const p1 = await createProvider('Gemini Provider', ['gemini', 'openai'], 1);
  const p2 = await createProvider('OpenRouter', ['openai', 'claude'], 2);
  const p3 = await createProvider('Claude Direct', ['claude'], 3);
  const p4 = await createProvider('Azure OpenAI', ['openai'], 4);

  // Create routes (position determines priority — lower number = tried first)
  await createRoute(p1, 'gemini', 1);
  await createRoute(p2, 'openai', 1);   // OpenRouter first for openai
  await createRoute(p1, 'openai', 2);   // Gemini Provider second
  await createRoute(p4, 'openai', 3);   // Azure third
  await createRoute(p3, 'claude', 1);   // Claude Direct first for claude
  await createRoute(p2, 'claude', 2);   // OpenRouter second for claude

  // Store PIDs for teardown
  process.env.MOCK_PID = String(mockProcess.pid);
  process.env.MAXX_PID = String(maxxProcess.pid);

  console.log(`[Setup] Done. Mock PID=${mockProcess.pid}, Maxx PID=${maxxProcess.pid}`);
}
