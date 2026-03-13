/**
 * Playwright 虚拟认证器 Passkey Discoverable Login 测试
 *
 * 使用方式：
 *   npx playwright test -c playwright.config.ts passkey-discoverable.spec.ts --project=e2e-chromium
 */
import { expect, test } from 'playwright/test';

import { BASE, bodyText, loginToAdminUI } from './helpers';

test.describe.configure({ mode: 'serial' });

async function waitForCredentialCount(
  cdp: { send: (method: string, params?: Record<string, unknown>) => Promise<{ credentials: Array<{ isResidentCredential?: boolean }> }> },
  authenticatorId: string,
  minimumCount: number,
  timeoutMs: number,
) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const result = await cdp.send('WebAuthn.getCredentials', { authenticatorId });
    if (result.credentials.length >= minimumCount) {
      return result.credentials;
    }
    await new Promise((resolve) => setTimeout(resolve, 200));
  }

  throw new Error(`Timed out waiting for ${minimumCount} WebAuthn credential(s)`);
}

test('passkey discoverable login works without username', async ({ browser }, testInfo) => {
  const context = await browser.newContext();
  const page = await context.newPage();

  try {
    const cdp = await context.newCDPSession(page);
    await cdp.send('WebAuthn.enable');
    const { authenticatorId } = await cdp.send('WebAuthn.addVirtualAuthenticator', {
      options: {
        protocol: 'ctap2',
        transport: 'internal',
        hasResidentKey: true,
        hasUserVerification: true,
        isUserVerified: true,
      },
    });

    console.log(`✅ Virtual authenticator added: ${authenticatorId}`);
    console.log(`   Target: ${BASE}`);

    console.log('\n--- Step 1: Password Login ---');
    await loginToAdminUI(page);
    console.log('✅ Password login success');

    console.log('\n--- Step 2: Register Passkey ---');
    const menuButton = page.locator('button[aria-haspopup="menu"]').last();
    await menuButton.click();

    const passkeyMenuItem = page.locator('[role="menuitem"]').filter({ hasText: /Passkey|passkey/ }).first();
    await expect(passkeyMenuItem).toBeVisible({ timeout: 5000 });
    await passkeyMenuItem.click();

    const dialog = page.locator('[role="dialog"]');
    await expect(dialog).toBeVisible({ timeout: 10000 });
    console.log('  Passkey Management dialog opened');

    const registerButton = page.locator('button').filter({ hasText: /Register Passkey|注册 Passkey/ }).first();
    await expect(registerButton).toBeVisible({ timeout: 5000 });
    await registerButton.click();

    await expect.poll(async () => (await dialog.textContent()) ?? '', { timeout: 10000 }).toContain('Passkey 1');

    const dialogPreview = ((await dialog.textContent()) ?? '').slice(0, 200);
    console.log(`  Dialog content preview: ${dialogPreview}`);

    const credentials = await waitForCredentialCount(cdp, authenticatorId, 1, 10000);
    console.log(`  Virtual authenticator credentials: ${credentials.length}`);
    expect(credentials.length).toBeGreaterThan(0);
    expect(credentials[0].isResidentCredential).toBeTruthy();
    console.log('✅ Credential is resident (discoverable)');

    const closeButton = dialog.locator('button').filter({ hasText: /Close|关闭/ }).first();
    if (await closeButton.isVisible()) {
      await closeButton.click();
      await expect(dialog).toBeHidden({ timeout: 5000 });
    }

    console.log('\n--- Step 3: Logout ---');
    const menuButtonAfter = page.locator('button[aria-haspopup="menu"]').last();
    await menuButtonAfter.click();

    const logoutItem = page.locator('[role="menuitem"]').filter({ hasText: /Log out|退出登录/ }).first();
    await expect(logoutItem).toBeVisible({ timeout: 5000 });
    await logoutItem.click();
    await expect(page.locator('input[type="text"]')).toBeVisible({ timeout: 5000 });
    console.log('✅ Logged out');

    console.log('\n--- Step 4: Discoverable Passkey Login (NO username) ---');
    const usernameField = page.locator('input[type="text"]');
    await usernameField.fill('');
    await expect(usernameField).toHaveValue('');
    console.log('  Username field is empty: ✓');

    const passkeyLoginButton = page.locator('button').filter({ hasText: /Login with Passkey|使用 Passkey 登录/ }).first();
    await expect(passkeyLoginButton).toBeEnabled();
    console.log('  Passkey login button is enabled without username: ✓');
    await passkeyLoginButton.click();

    await expect.poll(async () => /dashboard/i.test(await bodyText(page)), { timeout: 10000 }).toBe(true);
    console.log('✅ Discoverable passkey login SUCCESS!');

    console.log('\n--- Step 5: Verify logged-in state ---');
    await expect.poll(async () => /dashboard/i.test(await bodyText(page)), { timeout: 5000 }).toBe(true);
    console.log('✅ Dashboard visible - discoverable passkey login confirmed');

    const screenshot = await page.screenshot({ path: '/tmp/passkey-discoverable-result.png' });
    await testInfo.attach('passkey-discoverable-result', {
      body: screenshot,
      contentType: 'image/png',
    });
    console.log('  Screenshot: /tmp/passkey-discoverable-result.png');
  } finally {
    await context.close();
  }
});
