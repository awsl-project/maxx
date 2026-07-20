import { expect, test } from 'playwright/test';

import { BASE, PASS, USER, adminAPI, loginToAdminAPI } from './helpers';

test.describe.configure({ mode: 'serial' });

async function loginViaUI(page: import('playwright/test').Page, username: string, password: string) {
  await page.goto(BASE);
  await page.locator('input[type="text"]').fill(username);
  await page.locator('input[type="password"]').fill(password);
  await page.locator('button[type="submit"]').click();
  await expect(page.locator('body')).toContainText(/User Console|用户控制台/, { timeout: 15000 });
}

async function memberAPI(method: string, path: string, token: string, body?: unknown): Promise<any> {
  const response = await fetch(`${BASE}/api${path}`, {
    method,
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${token}`,
    },
    body: body ? JSON.stringify(body) : undefined,
  });
  const text = await response.text();
  let json: any = text;
  try {
    json = JSON.parse(text);
  } catch {
    // keep text for error reporting
  }
  if (!response.ok) {
    throw new Error(`Member API ${method} ${path} failed (${response.status}): ${text}`);
  }
  return json;
}

test('user panel key reveal uses a controlled endpoint and eye-toggle UI', async ({ page }, testInfo) => {
  const adminToken = await loginToAdminAPI();
  const username = `member-key-reveal-${Date.now()}`;
  const password = 'Member123!';
  let createdUserId: number | null = null;
  const previousSettings = await adminAPI('GET', '/settings', undefined, adminToken);

  try {
    await adminAPI('PUT', '/settings/ui_multitenant_enabled', { value: 'true' }, adminToken);
    await adminAPI('PUT', '/settings/ui_multitenant_layout', { value: 'user_panel' }, adminToken);

    const user = await adminAPI(
      'POST',
      '/users',
      {
        username,
        password,
        role: 'member',
      },
      adminToken,
    );
    createdUserId = user.id;

    await loginViaUI(page, username, password);
    await expect(page.getByText(/Generate|生成/)).toBeVisible({ timeout: 10000 });

    await page.getByRole('button', { name: /Generate|生成/ }).click();
    await expect(page.getByText(/Shown once|仅显示一次/)).toBeVisible({ timeout: 10000 });
    const createdBodyText = (await page.textContent('body')) ?? '';
    const createdFullKey = createdBodyText.match(/maxx_[0-9a-f]{64}/)?.[0] ?? '';
    expect(createdFullKey).toMatch(/^maxx_[0-9a-f]{64}$/);

    const createdScreenshot = '/tmp/maxx-user-panel-key-created-visible.png';
    await page.screenshot({ path: createdScreenshot, fullPage: true });
    await testInfo.attach('user-panel-key-created-visible', {
      path: createdScreenshot,
      contentType: 'image/png',
    });

    const memberLogin = await adminAPI('POST', '/auth/login', { username, password });
    const memberToken = memberLogin.token as string;
    expect(memberToken).toBeTruthy();

    const tokenMetadata = await memberAPI('GET', '/user-panel-token', memberToken);
    expect(tokenMetadata.apiToken.token).toBe('');
    expect(tokenMetadata.apiToken.tokenPrefix).toMatch(/^maxx_/);

    await page.reload();
    await expect(page.locator('body')).toContainText(/User Console|用户控制台/, { timeout: 15000 });
    const reloadedKeyInput = page.getByLabel(/Key|密钥/).first();
    await expect(reloadedKeyInput).toHaveAttribute('type', 'password');
    await expect(reloadedKeyInput).not.toHaveValue(createdFullKey);

    const hiddenScreenshot = '/tmp/maxx-user-panel-key-hidden-after-reload.png';
    await page.screenshot({ path: hiddenScreenshot, fullPage: true });
    await testInfo.attach('user-panel-key-hidden-after-reload', {
      path: hiddenScreenshot,
      contentType: 'image/png',
    });

    const revealed = await memberAPI('POST', '/user-panel-token/reveal', memberToken);
    expect(revealed.token).toBe(createdFullKey);

    await page.getByRole('button', { name: /Show key|显示密钥/ }).click();
    await expect(reloadedKeyInput).toHaveAttribute('type', 'text');
    await expect(reloadedKeyInput).toHaveValue(createdFullKey);
    await expect(page.getByText(/Full key is visible|完整密钥已显示/)).toBeVisible();

    const revealedScreenshot = '/tmp/maxx-user-panel-key-revealed-by-eye.png';
    await page.screenshot({ path: revealedScreenshot, fullPage: true });
    await testInfo.attach('user-panel-key-revealed-by-eye', {
      path: revealedScreenshot,
      contentType: 'image/png',
    });

    await page.getByRole('button', { name: /Hide key|隐藏密钥/ }).click();
    await expect(reloadedKeyInput).toHaveAttribute('type', 'password');
    await expect(reloadedKeyInput).not.toHaveValue(createdFullKey);
  } finally {
    await adminAPI(
      'PUT',
      '/settings/ui_multitenant_enabled',
      { value: previousSettings.ui_multitenant_enabled ?? 'false' },
      adminToken,
    ).catch(() => undefined);
    await adminAPI(
      'PUT',
      '/settings/ui_multitenant_layout',
      { value: previousSettings.ui_multitenant_layout ?? 'admin_panel' },
      adminToken,
    ).catch(() => undefined);
    if (createdUserId) {
      await adminAPI('DELETE', `/users/${createdUserId}`, undefined, adminToken).catch(() => undefined);
    }
  }
});
