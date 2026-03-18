import { expect, test } from 'playwright/test';

import { loginToAdminUI } from './helpers';

test.describe.configure({ mode: 'serial' });

test('expanded sidebar footer keeps compact single-line user chip', async ({ page }) => {
  await loginToAdminUI(page);

  const menuButton = page.locator('button[title="Menu"]');
  await expect(menuButton).toBeVisible({ timeout: 10000 });

  const footerCard = menuButton.locator('xpath=ancestor::div[contains(@class, "rounded-xl")]');
  await expect(footerCard).toContainText(/admin/i);
  await expect(footerCard).not.toContainText(/受保护|Protected/);
  await expect(footerCard).not.toContainText(/用户 .*租户|UID .*TID/);
});
