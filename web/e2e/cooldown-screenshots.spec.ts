/**
 * Cooldown UI Screenshots
 *
 * Uses a real maxx backend (port 19881) with test data.
 * Cooldowns are set/cleared via the admin API between scenarios.
 */
import { test, type Page } from '@playwright/test';

const BASE = 'http://localhost:19881';
const SCREENSHOT_DIR = 'e2e/screenshots';

// Provider IDs (created by setup script)
const LINKAPI = 1;
const OPENROUTER = 2;
const CLAUDE_DIRECT = 3;
const AZURE = 4;

function futureISO(minutes: number): string {
  return new Date(Date.now() + minutes * 60 * 1000).toISOString();
}

async function clearAllCooldowns() {
  for (const id of [LINKAPI, OPENROUTER, CLAUDE_DIRECT, AZURE]) {
    await fetch(`${BASE}/api/admin/cooldowns/${id}`, { method: 'DELETE' });
  }
}

async function setCooldown(providerId: number, minutes: number, clientType = '', model = '') {
  await fetch(`${BASE}/api/admin/cooldowns/${providerId}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ untilTime: futureISO(minutes), clientType, model }),
  });
}

async function navigateAndWait(page: Page, path: string) {
  await page.goto(`${BASE}${path}`);
  await page.waitForLoadState('networkidle');
  await page.waitForTimeout(2000);
}

test.use({ baseURL: BASE, viewport: { width: 1440, height: 900 } });

test.describe('Cooldown UI Screenshots', () => {
  test.beforeEach(async () => { await clearAllCooldowns(); });

  // ===== Providers Page =====

  test('01. Providers: All healthy', async ({ page }) => {
    await navigateAndWait(page, '/providers');
    await page.screenshot({ path: `${SCREENSHOT_DIR}/01-providers-healthy.png`, fullPage: true });
  });

  test('02. Providers: Provider frozen (network error)', async ({ page }) => {
    await setCooldown(OPENROUTER, 25); // provider-level
    await navigateAndWait(page, '/providers');
    await page.screenshot({ path: `${SCREENSHOT_DIR}/02-providers-frozen.png`, fullPage: true });
  });

  test('03. Providers: Single model frozen', async ({ page }) => {
    await setCooldown(LINKAPI, 5, 'gemini', 'gemini-2.5-flash-image');
    await navigateAndWait(page, '/providers');
    await page.screenshot({ path: `${SCREENSHOT_DIR}/03-providers-model-single.png`, fullPage: true });
  });

  test('04. Providers: Multiple models frozen', async ({ page }) => {
    await setCooldown(LINKAPI, 5, 'gemini', 'gemini-2.5-flash-image');
    await setCooldown(LINKAPI, 3, 'gemini', 'gemini-2.5-pro');
    await navigateAndWait(page, '/providers');
    await page.screenshot({ path: `${SCREENSHOT_DIR}/04-providers-model-multi.png`, fullPage: true });
  });

  test('05. Providers: Key-level rate limit', async ({ page }) => {
    await setCooldown(AZURE, 2, 'openai');
    await navigateAndWait(page, '/providers');
    await page.screenshot({ path: `${SCREENSHOT_DIR}/05-providers-key-limited.png`, fullPage: true });
  });

  test('06. Providers: All states combined', async ({ page }) => {
    await setCooldown(LINKAPI, 4, 'gemini', 'gemini-2.5-flash-image');  // model
    await setCooldown(OPENROUTER, 30);                                   // provider frozen
    await setCooldown(AZURE, 2, 'openai');                               // key limited
    await setCooldown(CLAUDE_DIRECT, 55);                                // provider frozen
    await navigateAndWait(page, '/providers');
    await page.screenshot({ path: `${SCREENSHOT_DIR}/06-providers-all-states.png`, fullPage: true });
  });

  // ===== Dashboard =====

  test('07. Dashboard: Healthy', async ({ page }) => {
    await navigateAndWait(page, '/');
    await page.screenshot({ path: `${SCREENSHOT_DIR}/07-dashboard-healthy.png`, fullPage: true });
  });

  test('08. Dashboard: Cooldown alert banner', async ({ page }) => {
    await setCooldown(OPENROUTER, 30);
    await setCooldown(AZURE, 5, 'openai', 'gpt-4o');
    await setCooldown(LINKAPI, 3, 'gemini', 'gemini-2.5-flash-image');
    await navigateAndWait(page, '/');
    await page.screenshot({ path: `${SCREENSHOT_DIR}/08-dashboard-cooldowns.png`, fullPage: true });
  });

  // ===== Routes Page =====

  test('09. Routes/Gemini: Model-level cooldown', async ({ page }) => {
    await setCooldown(LINKAPI, 5, 'gemini', 'gemini-2.5-flash-image');
    await navigateAndWait(page, '/routes/gemini');
    await page.screenshot({ path: `${SCREENSHOT_DIR}/09-routes-gemini-model.png`, fullPage: true });
  });

  test('10. Routes/OpenAI: Mixed cooldowns', async ({ page }) => {
    await setCooldown(OPENROUTER, 2, 'openai');                // key-level
    await setCooldown(AZURE, 5, 'openai', 'gpt-4o');          // model-level
    await navigateAndWait(page, '/routes/openai');
    await page.screenshot({ path: `${SCREENSHOT_DIR}/10-routes-openai-mixed.png`, fullPage: true });
  });
});
