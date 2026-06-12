/**
 * Cooldown UI Screenshots
 *
 * Uses maxx (:19881) + mock server (:19999) with session-based directives.
 * Each test creates a mock session, sets per-provider directives, then sends
 * proxy requests through maxx to trigger real cooldowns.
 */
import { test, type Page } from '@playwright/test';

const BASE = 'http://localhost:19881';
const MOCK = 'http://localhost:19999';
const SCREENSHOT_DIR = 'e2e/screenshots';

// Provider IDs (created in globalSetup, sequential from 1)
const P_GEMINI = '1';
const P_OPENROUTER = '2';
const P_AZURE = '4';

/** Set a mock directive for a specific provider (by ID). Returns session ID. */
async function mockSet(
  session: string | undefined,
  providerID: string,
  directive: object,
): Promise<string> {
  const resp = await fetch(`${MOCK}/__mock/set`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ session, providerID, directive }),
  });
  const data = await resp.json();
  return data.session;
}

/** Clear all mock directives for a session */
async function mockClear(session: string) {
  await fetch(`${MOCK}/__mock/clear`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ session }),
  });
}

/** Clear all cooldowns */
async function clearAllCooldowns() {
  for (const id of [1, 2, 3, 4]) {
    await fetch(`${BASE}/api/admin/cooldowns/${id}`, { method: 'DELETE' });
  }
}

/** Send a proxy request through maxx with a mock session */
async function proxyRequest(
  path: string,
  body: object,
  session: string,
) {
  await fetch(`${BASE}${path}`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'X-Mock-Session': session,
    },
    body: JSON.stringify(body),
  });
  // Brief wait for cooldown to be persisted
  await new Promise((r) => setTimeout(r, 200));
}

async function navigateAndWait(page: Page, path: string) {
  await page.goto(`${BASE}${path}`);
  await page.waitForLoadState('networkidle');
  await page.waitForTimeout(2000);
}

test.use({ baseURL: BASE, viewport: { width: 1440, height: 900 } });

test.describe('Cooldown UI Screenshots', () => {
  let session: string;

  test.beforeEach(async () => {
    await clearAllCooldowns();
    // Each test gets a fresh session
    session = await mockSet(undefined, '*', { status: 200 });
  });

  test.afterEach(async () => {
    await mockClear(session);
  });

  // ===== Providers Page =====

  test('01. Providers: All healthy', async ({ page }) => {
    await navigateAndWait(page, '/providers');
    await page.screenshot({ path: `${SCREENSHOT_DIR}/01-providers-healthy.png`, fullPage: true });
  });

  test('02. Providers: Provider frozen (502 from OpenRouter)', async ({ page }) => {
    // OpenRouter returns 502 → provider-level freeze
    await mockSet(session, P_OPENROUTER, { status: 502, headers: { 'Retry-After': '300' } });

    await proxyRequest(
      '/v1/chat/completions',
      { model: 'gpt-4o', messages: [{ role: 'user', content: 'hi' }] },
      session,
    );

    await navigateAndWait(page, '/providers');
    await page.screenshot({ path: `${SCREENSHOT_DIR}/02-providers-frozen.png`, fullPage: true });
  });

  test('03. Providers: Model-level 503 (gemini-2.5-flash-image overloaded)', async ({ page }) => {
    // Gemini provider returns 503 with model overloaded message
    await mockSet(session, P_GEMINI, {
      status: 503,
      headers: { 'Retry-After': '300' },
      body: { error: { message: 'Model gemini-2.5-flash-image is overloaded' } },
    });

    await proxyRequest(
      '/v1beta/models/gemini-2.5-flash-image:generateContent',
      { contents: [{ role: 'user', parts: [{ text: 'draw a cat' }] }] },
      session,
    );

    await navigateAndWait(page, '/providers');
    await page.screenshot({ path: `${SCREENSHOT_DIR}/03-providers-model-overload.png`, fullPage: true });
  });

  test('04. Providers: Key-level 429 rate limit (Azure)', async ({ page }) => {
    await mockSet(session, P_AZURE, {
      status: 429,
      headers: { 'Retry-After': '300' },
    });

    await proxyRequest(
      '/v1/chat/completions',
      { model: 'gpt-4o', messages: [{ role: 'user', content: 'hi' }] },
      session,
    );

    await navigateAndWait(page, '/providers');
    await page.screenshot({ path: `${SCREENSHOT_DIR}/04-providers-rate-limited.png`, fullPage: true });
  });

  test('05. Providers: Multi-provider different states', async ({ page }) => {
    // Gemini: 503 model overload
    await mockSet(session, P_GEMINI, {
      status: 503,
      headers: { 'Retry-After': '300' },
      body: { error: { message: 'Model gemini-2.5-flash-image is overloaded' } },
    });
    // OpenRouter: 502 provider down
    await mockSet(session, P_OPENROUTER, { status: 502, headers: { 'Retry-After': '300' } });
    // Azure: 429 rate limit
    await mockSet(session, P_AZURE, {
      status: 429,
      headers: { 'Retry-After': '300' },
    });
    // Claude Direct: 200 OK (default from wildcard)

    // Trigger errors
    await proxyRequest(
      '/v1beta/models/gemini-2.5-flash-image:generateContent',
      { contents: [{ role: 'user', parts: [{ text: 'test' }] }] },
      session,
    );
    await proxyRequest(
      '/v1/chat/completions',
      { model: 'gpt-4o', messages: [{ role: 'user', content: 'hi' }] },
      session,
    );

    await navigateAndWait(page, '/providers');
    await page.screenshot({ path: `${SCREENSHOT_DIR}/05-providers-multi.png`, fullPage: true });
  });

  // ===== Dashboard =====

  test('06. Dashboard: Cooldown alert banner', async ({ page }) => {
    await mockSet(session, P_OPENROUTER, { status: 502, headers: { 'Retry-After': '300' } });
    await mockSet(session, P_GEMINI, {
      status: 503,
      headers: { 'Retry-After': '300' },
      body: { error: { message: 'Model gemini-2.5-flash overloaded' } },
    });
    await mockSet(session, P_AZURE, {
      status: 429,
      headers: { 'Retry-After': '300' },
    });

    await proxyRequest(
      '/v1/chat/completions',
      { model: 'gpt-4o', messages: [{ role: 'user', content: 'hi' }] },
      session,
    );
    await proxyRequest(
      '/v1beta/models/gemini-2.5-flash:generateContent',
      { contents: [{ role: 'user', parts: [{ text: 'test' }] }] },
      session,
    );

    await navigateAndWait(page, '/');
    await page.screenshot({ path: `${SCREENSHOT_DIR}/06-dashboard-cooldowns.png`, fullPage: true });
  });

  // ===== Routes Page =====

  test('07. Routes/Gemini: Model cooldown', async ({ page }) => {
    await mockSet(session, P_GEMINI, {
      status: 503,
      headers: { 'Retry-After': '300' },
      body: { error: { message: 'Model gemini-2.5-flash-image overloaded' } },
    });

    await proxyRequest(
      '/v1beta/models/gemini-2.5-flash-image:generateContent',
      { contents: [{ role: 'user', parts: [{ text: 'test' }] }] },
      session,
    );

    await navigateAndWait(page, '/routes/gemini');
    await page.screenshot({ path: `${SCREENSHOT_DIR}/07-routes-gemini.png`, fullPage: true });

    // Click the provider row to open details dialog
    await page.click('text=Gemini Provider');
    await page.waitForTimeout(500);
    await page.screenshot({ path: `${SCREENSHOT_DIR}/07b-dialog-gemini.png` });
  });

  test('07c. Routes/Gemini: Multi-model cooldown + dialog', async ({ page }) => {
    // Two models frozen on same provider
    await mockSet(session, P_GEMINI, {
      status: 503,
      headers: { 'Retry-After': '300' },
      body: { error: { message: 'Model gemini-2.5-flash-image overloaded' } },
    });
    await proxyRequest(
      '/v1beta/models/gemini-2.5-flash-image:generateContent',
      { contents: [{ role: 'user', parts: [{ text: 'test' }] }] },
      session,
    );
    // Second model
    await mockSet(session, P_GEMINI, {
      status: 503,
      headers: { 'Retry-After': '300' },
      body: { error: { message: 'Model gemini-2.5-pro overloaded' } },
    });
    await proxyRequest(
      '/v1beta/models/gemini-2.5-pro:generateContent',
      { contents: [{ role: 'user', parts: [{ text: 'test2' }] }] },
      session,
    );

    await navigateAndWait(page, '/routes/gemini');
    await page.screenshot({ path: `${SCREENSHOT_DIR}/07c-routes-gemini-multi-model.png`, fullPage: true });

    // Open dialog
    await page.click('text=Gemini Provider');
    await page.waitForTimeout(500);
    await page.screenshot({ path: `${SCREENSHOT_DIR}/07d-dialog-gemini-multi-model.png` });
  });

  test('07e. Routes/Gemini: Unfreeze one model', async ({ page }) => {
    // Freeze two models
    await mockSet(session, P_GEMINI, {
      status: 503,
      headers: { 'Retry-After': '300' },
      body: { error: { message: 'Model gemini-2.5-flash-image overloaded' } },
    });
    await proxyRequest(
      '/v1beta/models/gemini-2.5-flash-image:generateContent',
      { contents: [{ role: 'user', parts: [{ text: 'test' }] }] },
      session,
    );
    await mockSet(session, P_GEMINI, {
      status: 503,
      headers: { 'Retry-After': '300' },
      body: { error: { message: 'Model gemini-2.5-pro overloaded' } },
    });
    await proxyRequest(
      '/v1beta/models/gemini-2.5-pro:generateContent',
      { contents: [{ role: 'user', parts: [{ text: 'test2' }] }] },
      session,
    );

    await navigateAndWait(page, '/routes/gemini');
    await page.screenshot({ path: `${SCREENSHOT_DIR}/07e-before-unfreeze.png`, fullPage: true });

    // Open dialog and unfreeze one model
    await page.click('text=Gemini Provider');
    await page.waitForTimeout(500);
    await page.screenshot({ path: `${SCREENSHOT_DIR}/07f-dialog-before-unfreeze.png` });

    // Click the first unfreeze button (Zap icon) for gemini-2.5-flash-image
    const unfreezeButtons = page.locator('[title="Unfreeze"]');
    await unfreezeButtons.first().click();
    await page.waitForTimeout(1000);
    await page.screenshot({ path: `${SCREENSHOT_DIR}/07g-dialog-after-unfreeze-one.png` });

    // Close dialog and see the routes page
    await page.keyboard.press('Escape');
    await page.waitForTimeout(500);
    await page.screenshot({ path: `${SCREENSHOT_DIR}/07h-routes-after-unfreeze-one.png`, fullPage: true });
  });

  test('08. Routes/OpenAI: Per-provider cooldowns', async ({ page }) => {
    // OpenRouter: 429, Azure: 503 model overload, Gemini Provider: OK
    await mockSet(session, P_OPENROUTER, {
      status: 429,
      headers: { 'Retry-After': '300' },
    });
    await mockSet(session, P_AZURE, {
      status: 503,
      headers: { 'Retry-After': '300' },
      body: { error: { message: 'Model gpt-4o is overloaded' } },
    });

    // Trigger errors — maxx will try routes in order
    await proxyRequest(
      '/v1/chat/completions',
      { model: 'gpt-4o', messages: [{ role: 'user', content: 'hi' }] },
      session,
    );

    await navigateAndWait(page, '/routes/openai');
    await page.screenshot({ path: `${SCREENSHOT_DIR}/08-routes-openai.png`, fullPage: true });

    // Click OpenRouter to open dialog
    await page.click('text=OpenRouter');
    await page.waitForTimeout(500);
    await page.screenshot({ path: `${SCREENSHOT_DIR}/08b-dialog-openrouter.png` });
  });
});
