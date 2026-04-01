/**
 * Cooldown UI Screenshots
 *
 * Uses a real maxx backend (port 19881) with mock server (port 19999).
 * Cooldowns are triggered by sending proxy requests with X-Mock-Response
 * headers that cause the mock server to return errors. Maxx classifies
 * these errors and sets cooldowns automatically.
 */
import { test, type Page } from '@playwright/test';

const BASE = 'http://localhost:19881';
const SCREENSHOT_DIR = 'e2e/screenshots';

// Provider IDs (created in globalSetup, sequential from 1)
const GEMINI_PROVIDER = 1;
const OPENROUTER = 2;
const CLAUDE_DIRECT = 3;
const AZURE = 4;

async function clearAllCooldowns() {
  for (const id of [GEMINI_PROVIDER, OPENROUTER, CLAUDE_DIRECT, AZURE]) {
    await fetch(`${BASE}/api/admin/cooldowns/${id}`, { method: 'DELETE' });
  }
}

/** Send a proxy request through maxx with a mock directive to trigger an error */
async function triggerError(path: string, body: object, mockDirective: object) {
  await fetch(`${BASE}${path}`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'X-Mock-Response': JSON.stringify(mockDirective),
    },
    body: JSON.stringify(body),
  });
}

async function navigateAndWait(page: Page, path: string) {
  await page.goto(`${BASE}${path}`);
  await page.waitForLoadState('networkidle');
  await page.waitForTimeout(2000);
}

test.use({ baseURL: BASE, viewport: { width: 1440, height: 900 } });

test.describe('Cooldown UI Screenshots', () => {
  test.beforeEach(async () => {
    await clearAllCooldowns();
  });

  // ===== Providers Page =====

  test('01. Providers: All healthy', async ({ page }) => {
    await navigateAndWait(page, '/providers');
    await page.screenshot({ path: `${SCREENSHOT_DIR}/01-providers-healthy.png`, fullPage: true });
  });

  test('02. Providers: Provider frozen via 502', async ({ page }) => {
    // Send request that triggers 502 (provider-level error)
    await triggerError(
      '/v1/chat/completions',
      { model: 'gpt-4o', messages: [{ role: 'user', content: 'hi' }] },
      { status: 502 },
    );
    await navigateAndWait(page, '/providers');
    await page.screenshot({ path: `${SCREENSHOT_DIR}/02-providers-frozen.png`, fullPage: true });
  });

  test('03. Providers: Model-level 503 overload', async ({ page }) => {
    // Send Gemini request that triggers 503 model overloaded
    await triggerError(
      '/v1beta/models/gemini-2.5-flash-image:generateContent',
      { contents: [{ role: 'user', parts: [{ text: 'draw a cat' }] }] },
      { status: 503, body: { error: { message: 'Model gemini-2.5-flash-image is overloaded' } } },
    );
    await navigateAndWait(page, '/providers');
    await page.screenshot({ path: `${SCREENSHOT_DIR}/03-providers-model-overload.png`, fullPage: true });
  });

  test('04. Providers: Key-level 429 rate limit', async ({ page }) => {
    await triggerError(
      '/v1/chat/completions',
      { model: 'gpt-4o', messages: [{ role: 'user', content: 'hi' }] },
      { status: 429, headers: { 'Retry-After': '30' } },
    );
    await navigateAndWait(page, '/providers');
    await page.screenshot({ path: `${SCREENSHOT_DIR}/04-providers-rate-limited.png`, fullPage: true });
  });

  test('05. Providers: Multiple errors', async ({ page }) => {
    // 503 model overload on Gemini provider
    await triggerError(
      '/v1beta/models/gemini-2.5-flash-image:generateContent',
      { contents: [{ role: 'user', parts: [{ text: 'test' }] }] },
      { status: 503, body: { error: { message: 'Model overloaded' } } },
    );
    // 429 on another OpenAI provider
    await triggerError(
      '/v1/chat/completions',
      { model: 'gpt-4o', messages: [{ role: 'user', content: 'hi' }] },
      { status: 429, headers: { 'Retry-After': '60' } },
    );
    await navigateAndWait(page, '/providers');
    await page.screenshot({ path: `${SCREENSHOT_DIR}/05-providers-multiple.png`, fullPage: true });
  });

  // ===== Dashboard =====

  test('06. Dashboard: Cooldown alert', async ({ page }) => {
    await triggerError(
      '/v1/chat/completions',
      { model: 'gpt-4o', messages: [{ role: 'user', content: 'hi' }] },
      { status: 429, headers: { 'Retry-After': '120' } },
    );
    await triggerError(
      '/v1beta/models/gemini-2.5-flash:generateContent',
      { contents: [{ role: 'user', parts: [{ text: 'test' }] }] },
      { status: 503, body: { error: { message: 'Model overloaded' } } },
    );
    await navigateAndWait(page, '/');
    await page.screenshot({ path: `${SCREENSHOT_DIR}/06-dashboard-cooldowns.png`, fullPage: true });
  });

  // ===== Routes Page =====

  test('07. Routes/Gemini: Model cooldown', async ({ page }) => {
    await triggerError(
      '/v1beta/models/gemini-2.5-flash-image:generateContent',
      { contents: [{ role: 'user', parts: [{ text: 'test' }] }] },
      { status: 503, body: { error: { message: 'Model gemini-2.5-flash-image overloaded' } } },
    );
    await navigateAndWait(page, '/routes/gemini');
    await page.screenshot({ path: `${SCREENSHOT_DIR}/07-routes-gemini.png`, fullPage: true });
  });

  test('08. Routes/OpenAI: Mixed cooldowns', async ({ page }) => {
    await triggerError(
      '/v1/chat/completions',
      { model: 'gpt-4o', messages: [{ role: 'user', content: 'hi' }] },
      { status: 429, headers: { 'Retry-After': '30' } },
    );
    await navigateAndWait(page, '/routes/openai');
    await page.screenshot({ path: `${SCREENSHOT_DIR}/08-routes-openai.png`, fullPage: true });
  });
});
