import { expect, test, type Locator, type Page } from '@playwright/test';

async function sectionTop(section: Locator) {
  const box = await section.boundingBox();
  expect(box).not.toBeNull();
  return box!.y;
}

function heading(page: Page, name: RegExp) {
  return page.getByRole('heading', { name }).first();
}

async function expectCustomProviderAdvancedSectionsCollapsed(page: Page) {
  const clientConfig = heading(page, /Client Configuration|Client protocols|客户端配置|客户端协议/);
  const advanced = page.locator('summary').filter({ hasText: /^Advanced settings$/ });
  const errorCooldown = heading(page, /Automatic Error Freeze|错误自动冻结/);
  const visibilityAndExport = heading(page, /Visibility and Export|Export & danger zone|可见性与导出|导出/);

  await expect(clientConfig).toBeVisible();
  await expect(advanced).toBeVisible();
  await expect(errorCooldown).not.toBeVisible();
  await expect(visibilityAndExport).not.toBeVisible();

  await advanced.click();

  await expect(errorCooldown).toBeVisible();
  await expect(visibilityAndExport).toBeVisible();
  expect(await sectionTop(visibilityAndExport)).toBeGreaterThan(await sectionTop(errorCooldown));
}

test.use({ viewport: { width: 1440, height: 1800 }, locale: 'zh-CN' });

test.describe('Custom provider advanced section layout', () => {
  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem('maxx-admin-token', 'mock-token');
      localStorage.setItem('maxx-ui-language', 'zh');
    });
  });

  test('create and edit flows keep advanced settings collapsed by default', async ({ page }) => {
    await page.goto('/providers/create/custom');
    await expectCustomProviderAdvancedSectionsCollapsed(page);

    await page.goto('/providers/1/edit');
    await expectCustomProviderAdvancedSectionsCollapsed(page);
  });
});
