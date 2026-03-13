/**
 * Playwright E2E Test: Project Update Should Not Lose TenantID
 *
 * 使用方式：
 *   npx playwright test -c playwright.config.ts project-update-tenant.spec.ts --project=e2e-chromium
 */
import { expect, test } from 'playwright/test';

import { BASE, adminAPI, loginToAdminAPI, loginToAdminUI } from './helpers';

test.describe.configure({ mode: 'serial' });

test('project updates preserve tenant visibility', async ({ browser }, testInfo) => {
  const jwt = await loginToAdminAPI();
  const context = await browser.newContext();
  const page = await context.newPage();

  let projectId: number | null = null;

  try {
    console.log('\n--- Setup: Create Project ---');
    const ts = Date.now();
    const projectName = `TenantTest-${ts}`;
    const project = await adminAPI('POST', '/projects', {
      name: projectName,
      enabledCustomRoutes: [],
    }, jwt);

    expect(project.id).toBeTruthy();
    expect(project.slug).toBeTruthy();
    projectId = project.id;
    console.log(`Project created: id=${project.id}, name=${project.name}, slug=${project.slug}`);

    const projectsBefore = await adminAPI('GET', '/projects', undefined, jwt);
    expect(projectsBefore.find((entry: any) => entry.id === project.id)).toBeTruthy();
    console.log('Project confirmed in API list');

    console.log('\n--- Step 1: Browser Login ---');
    await loginToAdminUI(page);
    console.log('Browser login success');

    console.log('\n--- Step 2: Navigate to Projects ---');
    await page.goto(`${BASE}/projects`);
    const projectCard = page.locator(`text=${projectName}`).first();
    await expect(projectCard).toBeVisible({ timeout: 10000 });
    console.log('Project visible in list');

    console.log('\n--- Step 3: Navigate to Project Detail ---');
    await projectCard.click();
    await expect(page.locator('input#name')).toBeVisible({ timeout: 10000 });
    await expect(page.locator('body')).toContainText(projectName);
    console.log('Project detail page loaded');

    console.log('\n--- Step 4: Toggle Custom Routes ---');
    const routesTab = page.locator('[data-slot="tabs-trigger"]').filter({ hasText: /Routes|路由/ }).first();
    await expect(routesTab).toBeVisible({ timeout: 10000 });
    await routesTab.click();

    const customRoutesSwitch = page.locator('[data-slot="switch"]').first();
    await expect(customRoutesSwitch).toBeVisible({ timeout: 5000 });

    await Promise.all([
      page.waitForResponse((response) => response.url().includes('/projects/') && response.request().method() === 'PUT', { timeout: 5000 }),
      customRoutesSwitch.click(),
    ]);
    console.log('Custom routes toggled');

    console.log('\n--- Step 5: Verify Project Still Exists via API ---');
    await expect.poll(async () => {
      const projects = await adminAPI('GET', '/projects', undefined, jwt);
      const found = projects.find((entry: any) => entry.id === project.id);
      return found?.enabledCustomRoutes?.length ?? 0;
    }, { timeout: 10000 }).toBeGreaterThan(0);

    const projectsAfterToggle = await adminAPI('GET', '/projects', undefined, jwt);
    const foundAfterToggle = projectsAfterToggle.find((entry: any) => entry.id === project.id);
    expect(foundAfterToggle).toBeTruthy();
    console.log(`Project still in API list, enabledCustomRoutes=${JSON.stringify(foundAfterToggle.enabledCustomRoutes)}`);

    console.log('\n--- Step 6: Navigate Back to Project List ---');
    await page.goto(`${BASE}/projects`);
    await expect(page.locator(`text=${projectName}`).first()).toBeVisible({ timeout: 10000 });
    console.log('Project still visible in list after toggle');

    console.log('\n--- Step 7: Edit Project Name in Overview ---');
    await page.locator(`text=${projectName}`).first().click();
    const nameInput = page.locator('input#name');
    await expect(nameInput).toBeVisible({ timeout: 10000 });

    const newName = `${projectName}-Edited`;
    await nameInput.fill(newName);

    const saveButton = page.locator('button').filter({ hasText: /Save|保存/ }).first();
    await expect(saveButton).toBeVisible({ timeout: 5000 });
    await Promise.all([
      page.waitForResponse((response) => response.url().includes('/projects/') && response.request().method() === 'PUT', { timeout: 5000 }),
      saveButton.click(),
    ]);

    await expect.poll(async () => {
      const projects = await adminAPI('GET', '/projects', undefined, jwt);
      return projects.find((entry: any) => entry.id === project.id)?.name ?? '';
    }, { timeout: 10000 }).toBe(newName);
    console.log(`Project name changed to: ${newName}`);

    console.log('\n--- Step 8: Verify Project Still Exists After Name Edit ---');
    const projectsAfterEdit = await adminAPI('GET', '/projects', undefined, jwt);
    const foundAfterEdit = projectsAfterEdit.find((entry: any) => entry.id === project.id);
    expect(foundAfterEdit).toBeTruthy();
    expect(foundAfterEdit.name).toBe(newName);
    console.log('Project still in API list with updated name');

    console.log('\n--- Step 9: Final Project List Check ---');
    await page.goto(`${BASE}/projects`);
    await expect(page.locator(`text=${newName}`).first()).toBeVisible({ timeout: 10000 });
    console.log('Project visible in final list check');

    const screenshot = await page.screenshot({ path: '/tmp/project-update-tenant-result.png' });
    await testInfo.attach('project-update-tenant-result', {
      body: screenshot,
      contentType: 'image/png',
    });
    console.log('Screenshot: /tmp/project-update-tenant-result.png');
  } finally {
    if (projectId) {
      try {
        await adminAPI('DELETE', `/projects/${projectId}`, undefined, jwt);
        console.log('Test project cleaned up');
      } catch (error) {
        console.warn('Failed to cleanup test project:', error instanceof Error ? error.message : String(error));
      }
    }
    await context.close();
  }
});
