import { expect, test } from '@playwright/test';

test('privacy module manages project policies without changing project settings', async ({
  page,
  request,
}, testInfo) => {
  const slug = `privacy-${Date.now()}`;
  const created = await request.post('/api/admin/projects', {
    data: { name: 'Credential protection', slug, enabledCustomRoutes: ['openai'] },
  });
  expect(created.status()).toBe(201);
  const project = await created.json();
  const otherCreated = await request.post('/api/admin/projects', {
    data: { name: 'Other project', slug: `${slug}-other` },
  });
  expect(otherCreated.status()).toBe(201);
  const other = await otherCreated.json();
  try {
    await page.addInitScript(() => {
      localStorage.setItem('maxx-admin-token', 'mock-token');
      localStorage.setItem('maxx-ui-language', 'zh');
    });
    await page.goto(`/projects/${project.id}`);
    await page.getByRole('link', { name: '管理隐私设置' }).click();
    await expect(page).toHaveURL(new RegExp(`/privacy\\?project=${project.id}$`));
    await expect(page.getByRole('heading', { name: '隐私保护', exact: true })).toBeVisible();
    const row = page.locator(`[data-project-id="${project.id}"]`);
    const control = row.getByRole('switch');
    await expect(control).not.toBeChecked();
    const saved = page.waitForResponse(
      (response) =>
        response.url().endsWith(`/projects/${project.id}/privacy`) &&
        response.request().method() === 'PUT',
    );
    await control.click();
    const response = await saved;
    expect(response.ok(), await response.text()).toBeTruthy();
    expect(response.request().postDataJSON()).toEqual({ blockSecrets: true });
    await expect(control).toBeChecked();
    await page.reload();
    await expect(control).toBeChecked();
    const stored = await (await request.get(`/api/admin/projects/${project.id}`)).json();
    expect(stored).toMatchObject({
      name: project.name,
      slug,
      blockSecrets: true,
      enabledCustomRoutes: ['openai'],
    });
    expect((await (await request.get(`/api/admin/projects/${other.id}`)).json()).blockSecrets).toBe(
      false,
    );

    await page.getByRole('button', { name: '显示所有项目' }).click();
    await expect(
      page.locator(`[data-project-id="${other.id}"]`).getByRole('switch'),
    ).not.toBeChecked();
    await page.evaluate(() => document.fonts.ready);
    await page.screenshot({ path: testInfo.outputPath('privacy-desktop.png'), fullPage: true });
    await page.setViewportSize({ width: 390, height: 844 });
    await page.screenshot({ path: testInfo.outputPath('privacy-mobile.png'), fullPage: true });
    await page.setViewportSize({ width: 1440, height: 1000 });

    const endpoint = `**/api/projects/${project.id}/privacy`;
    await page.route(endpoint, (route) =>
      route.fulfill({
        status: 500,
        contentType: 'application/json',
        body: '{"error":"test failure"}',
      }),
    );
    await control.click();
    await expect(page.getByRole('alert')).toHaveText('无法保存隐私设置，请重试。');
    await expect(control).toBeChecked();
    await page.unroute(endpoint);
    await control.click();
    await expect(control).not.toBeChecked();
    await page.reload();
    await expect(control).not.toBeChecked();
  } finally {
    await request.delete(`/api/admin/projects/${project.id}`);
    await request.delete(`/api/admin/projects/${other.id}`);
  }
});

test('full privacy editor previews and shows scoped history', async ({
  page,
  request,
}, testInfo) => {
  const slug = `full-privacy-${Date.now()}`;
  const created = await request.post('/api/admin/projects', {
    data: { name: 'Privacy workspace', slug },
  });
  expect(created.status()).toBe(201);
  const project = await created.json();
  try {
    await page.addInitScript(() => {
      localStorage.setItem('maxx-admin-token', 'mock-token');
      localStorage.setItem('maxx-ui-language', 'zh');
    });
    await page.goto(`/privacy?project=${project.id}`);
    await page.getByRole('switch', { name: '启用项目隐私保护', exact: true }).click();
    await page.getByRole('switch', { name: '个人信息识别', exact: true }).click();
    await page.getByRole('textbox', { name: '关键词', exact: true }).fill('Nightjar');
    await page.getByRole('switch', { name: '忽略大小写', exact: true }).click();
    const saved = page.waitForResponse(
      (response) =>
        response.url().endsWith(`/projects/${project.id}/privacy`) &&
        response.request().method() === 'PUT',
    );
    await page.getByRole('button', { name: '保存保护策略', exact: true }).click();
    expect((await saved).request().postDataJSON()).toMatchObject({
      config: { enabled: true, personal: true, words: ['Nightjar'], wordsAnyCase: true },
    });
    await page.reload();
    await expect(page.getByRole('textbox', { name: '关键词', exact: true })).toHaveValue(
      'Nightjar',
    );
    await expect(page.getByRole('switch', { name: '个人信息识别', exact: true })).toBeChecked();
    await page.locator('summary').filter({ hasText: '识别预览' }).click();
    await page
      .getByRole('textbox', { name: '待检查内容', exact: true })
      .fill('NIGHTJAR person@corp-real.com');
    await page.getByRole('button', { name: '运行预览', exact: true }).click();
    await expect(page.getByText('该请求可以发送，处理结果如下')).toBeVisible();
    await expect(page.locator('pre').first()).toContainText('{{TERM_');
    await expect(page.locator('pre').first()).not.toContainText('NIGHTJAR');
    expect(
      (await (await request.get(`/api/admin/projects/${project.id}/privacy/records`)).json()).total,
    ).toBe(0);
    await page.screenshot({
      path: testInfo.outputPath('privacy-preview-desktop.png'),
      fullPage: true,
    });
    await page.getByText('Privacy workspace · 保护设置', { exact: true }).scrollIntoViewIfNeeded();
    await page.screenshot({
      path: testInfo.outputPath('privacy-full-desktop.png'),
      fullPage: true,
    });
    await page.setViewportSize({ width: 390, height: 844 });
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
    ).toBeTruthy();
    await page.getByText('Privacy workspace · 保护设置', { exact: true }).scrollIntoViewIfNeeded();
    await page.screenshot({ path: testInfo.outputPath('privacy-full-mobile.png'), fullPage: true });
    await page.setViewportSize({ width: 1440, height: 1000 });
    await page.locator('summary').filter({ hasText: '本地识别模型' }).click();
    await expect(page.getByText('astrlink-guard', { exact: true })).toBeVisible();
    await page.getByText('astrlink-guard', { exact: true }).scrollIntoViewIfNeeded();
    await page.screenshot({
      path: testInfo.outputPath('privacy-models-desktop.png'),
      fullPage: true,
    });
    const proxy = await request.post(`/project/${slug}/v1/chat/completions`, {
      data: { model: 'gpt-4o', messages: [{ role: 'user', content: 'Nightjar' }] },
    });
    expect(proxy.status()).toBe(200);
    await page.getByRole('button', { name: '命中记录', exact: true }).click();
    await expect(page.getByRole('button', { name: '详情', exact: true }).first()).toBeVisible();
    await page.getByRole('button', { name: '详情', exact: true }).first().click();
    await page.getByRole('button', { name: '查看原值', exact: true }).first().click();
    await expect(page.locator('pre').first()).toHaveText('Nightjar');
    await page.screenshot({
      path: testInfo.outputPath('privacy-history-desktop.png'),
      fullPage: true,
    });
    page.once('dialog', (dialog) => dialog.accept());
    await page.getByRole('button', { name: '清理记录与记忆', exact: true }).click();
    await expect(page.getByText('暂无命中记录')).toBeVisible();
    await expect(page.locator('pre')).toHaveCount(0);
  } finally {
    await request.delete(`/api/admin/projects/${project.id}`);
  }
});
