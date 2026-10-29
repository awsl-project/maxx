import { expect, test } from '@playwright/test';

test.describe.configure({ mode: 'serial' });

test('provider exemption history renders empty findings and outbound details', async ({ page, request }) => {
  const original = await (await request.get('/api/admin/privacy')).json();
  const providers = await (await request.get('/api/admin/providers')).json();
  const provider = providers.find((p: { supportedClientTypes: string[] }) => p.supportedClientTypes.includes('openai'));
  expect(provider).toBeTruthy();
  try {
    await request.delete('/api/admin/privacy/records');
    expect((await request.put('/api/admin/privacy', {
      data: { config: { ...original, enabled: true, bypassProviders: [provider.id] } },
    })).ok()).toBeTruthy();
    expect((await request.post(`/provider/${provider.id}/v1/chat/completions`, {
      data: { model: 'gpt-4o', messages: [{ role: 'user', content: 'synthetic exemption request' }] },
    })).status()).toBe(200);
    await expect.poll(async () => (await (await request.get('/api/admin/privacy/records')).json()).total).toBe(1);
    const records = await (await request.get('/api/admin/privacy/records')).json();
    expect(records.items[0].bypassed).toBe(true);
    expect(records.items[0].values).toEqual([]);
    const errors: string[] = [];
    page.on('pageerror', (error) => errors.push(error.message));
    await page.addInitScript(() => {
      localStorage.setItem('maxx-admin-token', 'mock-token');
      localStorage.setItem('maxx-ui-language', 'zh');
    });
    await page.goto('/privacy');
    await page.getByRole('button', { name: '命中记录', exact: true }).click();
    await page.getByRole('button', { name: '详情', exact: true }).click();
    await expect(page.getByRole('button', { name: '查看原值', exact: true })).toHaveCount(0);
    await page.getByRole('button', { name: '查看出站正文', exact: true }).click();
    await expect(page.locator('pre')).toContainText('synthetic exemption request');
    expect(errors).toEqual([]);
  } finally {
    await request.put('/api/admin/privacy', { data: { config: original } });
    await request.delete('/api/admin/privacy/records');
  }
});

test('global privacy persists and projects have no policy override', async ({ page, request }) => {
  const original = await (await request.get('/api/admin/privacy')).json();
  const created = await request.post('/api/admin/projects', {
    data: { name: 'Global privacy fixture', slug: `global-privacy-${Date.now()}` },
  });
  const project = await created.json();
  try {
    expect(project).not.toHaveProperty('blockSecrets');
    expect(project).not.toHaveProperty('privacy');
    await page.addInitScript(() => {
      localStorage.setItem('maxx-admin-token', 'mock-token');
      localStorage.setItem('maxx-ui-language', 'zh');
    });
    await page.goto('/privacy');
    const control = page.getByRole('switch', { name: '启用全局隐私保护', exact: true });
    await expect(control).not.toBeChecked();
    await control.click();
    const saved = page.waitForResponse(
      (r) => r.url().endsWith('/api/admin/privacy') && r.request().method() === 'PUT',
    );
    await page.getByRole('button', { name: '保存保护策略', exact: true }).click();
    expect((await saved).ok()).toBeTruthy();
    await page.reload();
    await expect(control).toBeChecked();

    expect((await request.get(`/api/admin/projects/${project.id}/privacy`)).status()).toBe(404);
    expect((await request.get(`/api/projects/${project.id}/privacy`)).status()).toBe(404);
    await request.put(`/api/admin/projects/${project.id}`, {
      data: { name: project.name, slug: project.slug },
    });
    expect((await (await request.get('/api/admin/privacy')).json()).enabled).toBe(true);

    await page.route('**/api/admin/privacy', (route) =>
      route.request().method() === 'PUT'
        ? route.fulfill({
            status: 500,
            contentType: 'application/json',
            body: '{"error":"test failure"}',
          })
        : route.continue(),
    );
    await control.click();
    await page.getByRole('button', { name: '保存保护策略', exact: true }).click();
    await expect(page.getByRole('alert')).toHaveText('无法保存隐私设置，请重试。');
    expect((await (await request.get('/api/admin/privacy')).json()).enabled).toBe(true);
    await page.unroute('**/api/admin/privacy');
  } finally {
    await request.put('/api/admin/privacy', { data: { config: original } });
    await request.delete(`/api/admin/projects/${project.id}`);
  }
});

test('global privacy previews and records unassigned and project requests', async ({
  page,
  request,
}, testInfo) => {
  const original = await (await request.get('/api/admin/privacy')).json();
  const created = await request.post('/api/admin/projects', {
    data: { name: 'Global scope fixture', slug: `global-scope-${Date.now()}` },
  });
  const project = await created.json();
  try {
    await request.delete('/api/admin/privacy/records');
    await page.addInitScript(() => {
      localStorage.setItem('maxx-admin-token', 'mock-token');
      localStorage.setItem('maxx-ui-language', 'zh');
    });
    await page.goto('/privacy');
    await page.getByRole('switch', { name: '启用全局隐私保护', exact: true }).click();
    await page.getByRole('switch', { name: '个人信息识别', exact: true }).click();
    await page.getByRole('textbox', { name: '关键词', exact: true }).fill('Nightjar');
    await page.getByRole('switch', { name: '忽略大小写', exact: true }).click();
    const saved = page.waitForResponse(
      (r) => r.url().endsWith('/api/admin/privacy') && r.request().method() === 'PUT',
    );
    await page.getByRole('button', { name: '保存保护策略', exact: true }).click();
    expect((await saved).ok()).toBeTruthy();
    await page.reload();
    await expect(page.getByRole('textbox', { name: '关键词', exact: true })).toHaveValue(
      'Nightjar',
    );
    await page.locator('summary').filter({ hasText: '识别预览' }).click();
    await page
      .getByRole('textbox', { name: '待检查内容', exact: true })
      .fill('NIGHTJAR person@corp-real.com');
    await page.getByRole('button', { name: '运行预览', exact: true }).click();
    await expect(page.locator('pre').first()).toContainText('{{TERM_');
    await expect(page.locator('pre').first()).not.toContainText('NIGHTJAR');
    expect((await (await request.get('/api/admin/privacy/records')).json()).total).toBe(0);
    await page.screenshot({
      path: testInfo.outputPath('privacy-global-desktop.png'),
      fullPage: true,
    });
    await page.setViewportSize({ width: 390, height: 844 });
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),
    ).toBeTruthy();
    await page.screenshot({
      path: testInfo.outputPath('privacy-global-mobile.png'),
      fullPage: true,
    });
    await page.setViewportSize({ width: 1440, height: 1000 });

    for (const path of ['/v1/chat/completions', `/project/${project.slug}/v1/chat/completions`]) {
      const response = await request.post(path, {
        data: { model: 'gpt-4o', messages: [{ role: 'user', content: 'Nightjar' }] },
      });
      expect(response.status()).toBe(200);
    }
    expect((await (await request.get('/api/admin/privacy/records')).json()).total).toBe(2);
    await page.getByRole('button', { name: '命中记录', exact: true }).click();
    await page.getByRole('button', { name: '详情', exact: true }).first().click();
    await page.getByRole('button', { name: '查看原值', exact: true }).first().click();
    await expect(page.locator('pre').first()).toHaveText('Nightjar');
    await page.screenshot({
      path: testInfo.outputPath('privacy-global-history.png'),
      fullPage: true,
    });
    page.once('dialog', (dialog) => dialog.accept());
    await page.getByRole('button', { name: '清理记录与记忆', exact: true }).click();
    await expect(page.getByText('暂无命中记录')).toBeVisible();
    await expect(page.locator('pre')).toHaveCount(0);
  } finally {
    await request.put('/api/admin/privacy', { data: { config: original } });
    await request.delete(`/api/admin/projects/${project.id}`);
  }
});
