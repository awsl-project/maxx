import { expect, test } from '@playwright/test';

test('project credential protection persists across save and reload', async ({ page, request }) => {
  const slug = `privacy-${Date.now()}`;
  const created = await request.post('/api/admin/projects', {
    data: { name: 'Credential protection', slug },
  });
  expect(created.status()).toBe(201);
  const project = await created.json();
  expect(project.blockSecrets).toBe(false);

  await page.addInitScript(() => {
    localStorage.setItem('maxx-admin-token', 'mock-token');
    localStorage.setItem('maxx-ui-language', 'zh');
  });
  await page.goto(`/projects/${project.id}`);
  const control = page.getByRole('switch', { name: '拦截消息中的凭据' });
  await expect(control).not.toBeChecked();
  await control.click();
  await page.getByRole('button', { name: '保存更改' }).click();
  await expect
    .poll(
      async () =>
        (await (await request.get(`/api/admin/projects/${project.id}`)).json()).blockSecrets,
    )
    .toBe(true);
  await page.reload();
  await expect(control).toBeChecked();

  await control.click();
  await page.getByRole('button', { name: '保存更改' }).click();
  await expect
    .poll(
      async () =>
        (await (await request.get(`/api/admin/projects/${project.id}`)).json()).blockSecrets,
    )
    .toBe(false);
  await page.reload();
  await expect(control).not.toBeChecked();
  await request.delete(`/api/admin/projects/${project.id}`);
});
