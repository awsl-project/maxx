import { expect, test } from 'playwright/test';

import { loginToAdminUI } from './helpers';

test.describe.configure({ mode: 'serial' });

test('expanded sidebar footer separates account summary from utility actions', async ({ page }) => {
  await loginToAdminUI(page);

  const settingsButton = page.locator('button[title="Settings"], button[title="设置"]');
  await expect(settingsButton).toBeVisible({ timeout: 10000 });

  const footerCard = settingsButton.locator('xpath=ancestor::div[contains(@class, "rounded-xl")]');
  await expect(footerCard).toContainText(/admin/i);
  await expect(footerCard).toContainText(/GitHub/i);
  await expect(footerCard).not.toContainText(/受保护|Protected/);
  await expect(footerCard).not.toContainText(/用户 .*租户|UID .*TID/);
});
