import { execSync, spawn, type ChildProcess } from 'child_process';
import { mkdtempSync } from 'fs';
import { tmpdir } from 'os';
import { join } from 'path';

const MAXX_PORT = 19881;
const MOCK_PORT = 19999;
const MAXX_URL = `http://localhost:${MAXX_PORT}`;
const MOCK_URL = `http://localhost:${MOCK_PORT}`;

let mockProcess: ChildProcess;
let maxxProcess: ChildProcess;

async function waitForServer(url: string, timeoutMs = 15000): Promise<void> {
  const start = Date.now();
  while (Date.now() - start < timeoutMs) {
    try {
      const resp = await fetch(url);
      if (resp.ok) return;
    } catch {}
    await new Promise((r) => setTimeout(r, 200));
  }
  throw new Error(`Server at ${url} not ready after ${timeoutMs}ms`);
}

async function createProvider(name: string, types: string[]) {
  const resp = await fetch(`${MAXX_URL}/api/admin/providers`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      type: 'custom',
      name,
      supportedClientTypes: types,
      config: { custom: { baseURL: MOCK_URL, apiKey: 'mock-key' } },
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
  const projectRoot = join(__dirname, '..', '..');

  // Build mock server
  execSync('go build -o /tmp/maxx-mockserver ./cmd/mockserver', {
    cwd: projectRoot,
    stdio: 'pipe',
  });

  // Build maxx
  execSync('go build -o /tmp/maxx-test ./cmd/maxx', {
    cwd: projectRoot,
    stdio: 'pipe',
  });

  // Start mock server
  mockProcess = spawn('/tmp/maxx-mockserver', ['-addr', `:${MOCK_PORT}`], {
    stdio: 'pipe',
  });

  // Start maxx with temp data dir
  const tmpDir = mkdtempSync(join(tmpdir(), 'maxx-e2e-'));
  maxxProcess = spawn('/tmp/maxx-test', ['-addr', `:${MAXX_PORT}`, '-data', tmpDir], {
    stdio: 'pipe',
  });

  // Wait for both servers
  await waitForServer(`${MOCK_URL}/v1/chat/completions`);
  await waitForServer(`${MAXX_URL}/health`);

  // Create test providers pointing to mock server
  const p1 = await createProvider('Gemini Provider', ['gemini', 'openai']);
  const p2 = await createProvider('OpenRouter', ['openai', 'claude']);
  const p3 = await createProvider('Claude Direct', ['claude']);
  const p4 = await createProvider('Azure OpenAI', ['openai']);

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
}
