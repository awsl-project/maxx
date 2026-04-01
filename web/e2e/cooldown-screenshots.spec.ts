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

// ============================================================
// API helpers
// ============================================================

async function clearAllCooldowns() {
  for (const id of [LINKAPI, OPENROUTER, CLAUDE_DIRECT, AZURE]) {
    await fetch(`${BASE}/api/admin/cooldowns/${id}`, { method: 'DELETE' });
  }
}

async function setCooldown(
  providerId: number,
  minutes: number,
  clientType = '',
  model = '',
) {
  await fetch(`${BASE}/api/admin/cooldowns/${providerId}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      untilTime: futureISO(minutes),
      clientType,
      model,
    }),
  });
}

// ============================================================
// Helpers
// ============================================================

async function navigateAndWait(page: Page, path: string) {
  await page.goto(`${BASE}${path}`);
  await page.waitForLoadState('networkidle');
  await page.waitForTimeout(2000);
}

// ============================================================
// Tests
// ============================================================

test.use({
  baseURL: BASE,
  viewport: { width: 1440, height: 900 },
});

test.describe('Cooldown UI Screenshots', () => {

  test.beforeEach(async () => {
    await clearAllCooldowns();
  });

  // --- Scenario 1: All healthy ---
  test('01. Providers: All healthy', async ({ page }) => {
    await navigateAndWait(page, '/providers');
    await page.screenshot({ path: `${SCREENSHOT_DIR}/01-providers-all-healthy.png`, fullPage: true });
  });

  // --- Scenario 2: Provider fully frozen (network error) ---
  test('02. Providers: Provider fully frozen', async ({ page }) => {
    await setCooldown(LINKAPI, 25); // provider-level (no clientType, no model)
    await navigateAndWait(page, '/providers');
    await page.screenshot({ path: `${SCREENSHOT_DIR}/02-providers-provider-frozen.png`, fullPage: true });
  });

  // --- Scenario 3: Model-level cooldown only ---
  test('03. Providers: Model-level cooldown (degraded)', async ({ page }) => {
    await setCooldown(LINKAPI, 4, 'gemini', 'gemini-2.5-flash-image');
    await navigateAndWait(page, '/providers');
    await page.screenshot({ path: `${SCREENSHOT_DIR}/03-providers-model-frozen.png`, fullPage: true });
  });

  // --- Scenario 4: Key-level rate limit ---
  test('04. Providers: Key-level rate limit (limited)', async ({ page }) => {
    await setCooldown(OPENROUTER, 2, 'openai');
    await navigateAndWait(page, '/providers');
    await page.screenshot({ path: `${SCREENSHOT_DIR}/04-providers-key-limited.png`, fullPage: true });
  });

  // --- Scenario 5: Mixed cooldowns on one provider ---
  test('05. Providers: Mixed model cooldowns', async ({ page }) => {
    await setCooldown(LINKAPI, 4, 'gemini', 'gemini-2.5-flash-image');
    await setCooldown(LINKAPI, 1, 'gemini', 'gemini-2.5-pro');
    await navigateAndWait(page, '/providers');
    await page.screenshot({ path: `${SCREENSHOT_DIR}/05-providers-mixed.png`, fullPage: true });
  });

  // --- Scenario 6: Multiple providers affected ---
  test('06. Providers: Multiple providers affected', async ({ page }) => {
    await setCooldown(LINKAPI, 30);               // provider-level freeze
    await setCooldown(OPENROUTER, 2, 'openai');    // key-level
    await setCooldown(CLAUDE_DIRECT, 55);          // provider-level
    await setCooldown(AZURE, 5, 'openai', 'gpt-4o'); // model-level
    await navigateAndWait(page, '/providers');
    await page.screenshot({ path: `${SCREENSHOT_DIR}/06-providers-multi-affected.png`, fullPage: true });
  });

  // --- Scenario 7: Overview with cooldowns ---
  test('07. Overview: Multiple cooldowns', async ({ page }) => {
    await setCooldown(LINKAPI, 30);
    await setCooldown(OPENROUTER, 2, 'openai');
    await setCooldown(AZURE, 5, 'openai', 'gpt-4o');
    await navigateAndWait(page, '/');
    await page.screenshot({ path: `${SCREENSHOT_DIR}/07-overview-multi-cooldowns.png`, fullPage: true });
  });

  // --- Scenario 8: Overview all healthy ---
  test('08. Overview: All healthy', async ({ page }) => {
    await navigateAndWait(page, '/');
    await page.screenshot({ path: `${SCREENSHOT_DIR}/08-overview-healthy.png`, fullPage: true });
  });

  // --- Scenario 9: Routes page with model cooldown ---
  test('09. Routes: Gemini with model-level cooldown', async ({ page }) => {
    await setCooldown(LINKAPI, 4, 'gemini', 'gemini-2.5-flash-image');
    await navigateAndWait(page, '/routes/gemini');
    await page.screenshot({ path: `${SCREENSHOT_DIR}/09-routes-gemini-model-frozen.png`, fullPage: true });
  });

  // --- Scenario 10: Routes page with multiple cooldowns ---
  test('10. Routes: OpenAI with mixed cooldowns', async ({ page }) => {
    await setCooldown(OPENROUTER, 2, 'openai');
    await setCooldown(AZURE, 5, 'openai', 'gpt-4o');
    await navigateAndWait(page, '/routes/openai');
    await page.screenshot({ path: `${SCREENSHOT_DIR}/10-routes-openai-multi.png`, fullPage: true });
  });
});
