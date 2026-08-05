import { expect, test, type Locator, type Page } from '@playwright/test';

async function sectionTop(section: Locator) {
  const box = await section.boundingBox();
  expect(box).not.toBeNull();
  return box!.y;
}

function heading(page: Page, name: RegExp) {
  return page.getByRole('heading', { name }).first();
}

async function expectCustomProviderSectionOrder(page: Page) {
  const clientConfig = heading(page, /Client Configuration|客户端配置/);
  const errorCooldown = heading(page, /Automatic Error Freeze|错误自动冻结/);
  const visibilityAndExport = heading(page, /Visibility and Export|可见性与导出/);

  await expect(clientConfig).toBeVisible();
  await expect(errorCooldown).toBeVisible();
  await expect(visibilityAndExport).toBeVisible();

  expect(await sectionTop(errorCooldown)).toBeGreaterThan(await sectionTop(clientConfig));
  expect(await sectionTop(visibilityAndExport)).toBeGreaterThan(await sectionTop(errorCooldown));
}

test.use({ viewport: { width: 1440, height: 1800 }, locale: 'zh-CN' });

test.describe('Custom provider visibility/export section order', () => {
  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem('maxx-admin-token', 'mock-token');
      localStorage.setItem('maxx-ui-language', 'zh');
    });
  });

  test('edit flow keeps visibility/export in the same position as create flow', async ({ page }) => {
    await page.goto('/providers/create/custom');
    await expectCustomProviderSectionOrder(page);

    await page.goto('/providers/1/edit');
    await expectCustomProviderSectionOrder(page);
  });
});
