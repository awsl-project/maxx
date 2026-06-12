import { expect, test } from 'playwright/test';

import { loginToAdminUI } from './helpers';

test.describe.configure({ mode: 'serial' });

test('expanded sidebar footer keeps language github and settings in one aligned row', async ({ page }) => {
  await loginToAdminUI(page);

  const footerCard = page
    .locator('[data-footer-actions="true"]')
    .locator('xpath=ancestor::div[contains(@class, "rounded-xl")]');
  const settingsButton = footerCard.getByRole('button', { name: /settings|设置/i });
  const githubLink = footerCard.getByRole('link', { name: /github/i });
  const languageButton = footerCard.getByRole('button', { name: /language|语言/i });

  await expect(settingsButton).toBeVisible({ timeout: 10000 });
  await expect(githubLink).toBeVisible();
  await expect(languageButton).toBeVisible();

  const [githubText, settingsText] = await Promise.all([
    githubLink.evaluate((node) => node.textContent?.trim() ?? ''),
    settingsButton.evaluate((node) => node.textContent?.trim() ?? ''),
  ]);
  expect(githubText).toBe('');
  expect(settingsText).toBe('');

  const boxes = await Promise.all([
    languageButton.boundingBox(),
    githubLink.boundingBox(),
    settingsButton.boundingBox(),
  ]);
  for (const box of boxes) {
    expect(box).not.toBeNull();
  }
  const [languageBox, githubBox, settingsBox] = boxes as NonNullable<(typeof boxes)[number]>[];
  expect(Math.abs(languageBox.y - githubBox.y)).toBeLessThanOrEqual(1);
  expect(Math.abs(githubBox.y - settingsBox.y)).toBeLessThanOrEqual(1);
  expect(githubBox.x).toBeLessThan(languageBox.x);
  expect(languageBox.x).toBeLessThan(settingsBox.x);
});
