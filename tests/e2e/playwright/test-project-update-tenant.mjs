/**
 * Playwright E2E Test: Project Update Should Not Lose TenantID
 *
 * Bug: When updating a project (e.g. toggling custom routes or saving overview),
 * the PUT handler did not preserve TenantID, causing it to be set to 0.
 * This made the project disappear from the list (filtered by tenant_id).
 *
 * Test flow:
 * 1. Admin login via API
 * 2. Create a project via API
 * 3. Browser login, navigate to project detail
 * 4. Toggle custom routes on (triggers PUT update)
 * 5. Navigate back to project list and verify project still exists
 * 6. Navigate to project detail again and edit name/slug (triggers PUT update)
 * 7. Navigate back to project list and verify project still exists
 *
 * Usage:
 *   node test-project-update-tenant.mjs [base_url] [username] [password]
 *
 *   Defaults:
 *     base_url = http://localhost:9880
 *     username = admin
 *     password = test123
 */
import { chromium } from 'playwright';

const BASE = process.argv[2] || 'http://localhost:9880';
const USER = process.argv[3] || 'admin';
const PASS = process.argv[4] || 'test123';
const HEADED = !!process.env.HEADED;

let exitCode = 0;
let browser = null;
let projectId = null;
let jwt = null;

function assert(condition, msg) {
  if (!condition) {
    console.error(`ASSERTION FAILED: ${msg}`);
    exitCode = 1;
    throw new Error(msg);
  }
}

// ===== Admin API Helper =====
async function adminAPI(method, path, body, token) {
  const url = `${BASE}/api/admin${path}`;
  const headers = { 'Content-Type': 'application/json' };
  if (token) headers['Authorization'] = `Bearer ${token}`;

  const res = await fetch(url, {
    method,
    headers,
    body: body ? JSON.stringify(body) : undefined,
  });

  const text = await res.text();
  let json;
  try {
    json = JSON.parse(text);
  } catch {
    json = text;
  }

  if (!res.ok) {
    throw new Error(`Admin API ${method} ${path} failed (${res.status}): ${text}`);
  }
  return json;
}

// ===== Cleanup Helper =====
async function cleanup() {
  if (projectId && jwt) {
    try {
      await adminAPI('DELETE', `/projects/${projectId}`, null, jwt);
      console.log('Test project cleaned up');
    } catch (e) {
      console.warn('Failed to cleanup test project:', e.message);
    }
  }
  if (browser) {
    try { await browser.close(); } catch {}
  }
}

// ===== Main Test =====
(async () => {
  // --- Setup: Admin login ---
  console.log('\n--- Setup: Admin Login ---');
  const loginResp = await adminAPI('POST', '/auth/login', {
    username: USER,
    password: PASS,
  });
  assert(loginResp.token, 'Should receive JWT token');
  jwt = loginResp.token;
  console.log('Admin login success');

  // --- Setup: Create Project ---
  console.log('\n--- Setup: Create Project ---');
  const ts = Date.now();
  const projectName = `TenantTest-${ts}`;
  const project = await adminAPI(
    'POST',
    '/projects',
    {
      name: projectName,
      enabledCustomRoutes: [],
    },
    jwt,
  );
  assert(project.id, 'Project should have an ID');
  assert(project.slug, 'Project should have a slug');
  projectId = project.id;
  console.log(`Project created: id=${project.id}, name=${project.name}, slug=${project.slug}`);

  // Verify project appears in list via API
  const projectsBefore = await adminAPI('GET', '/projects', null, jwt);
  const foundBefore = projectsBefore.find((p) => p.id === project.id);
  assert(foundBefore, 'Project should appear in API list after creation');
  console.log('Project confirmed in API list');

  // --- Browser Test ---
  console.log('\n--- Browser: Launch ---');
  browser = await chromium.launch({ headless: !HEADED });
  const context = await browser.newContext();
  const page = await context.newPage();

  // Step 1: Login
  console.log('\n--- Step 1: Browser Login ---');
  await page.goto(BASE);
  await page.waitForSelector('input[type="text"]', { timeout: 10000 });
  await page.fill('input[type="text"]', USER);
  await page.fill('input[type="password"]', PASS);
  await page.locator('button[type="submit"]').click();
  await page.waitForURL('**/(dashboard|projects|stats|routes)**', { timeout: 10000 });
  console.log('Browser login success');

  // Step 2: Navigate to projects page and verify project visible
  console.log('\n--- Step 2: Navigate to Projects ---');
  await page.goto(`${BASE}/projects`);
  const projectCard = page.locator(`text=${projectName}`);
  await projectCard.first().waitFor({ state: 'visible', timeout: 10000 });
  console.log('Project visible in list');

  // Step 3: Click project to go to detail page
  console.log('\n--- Step 3: Navigate to Project Detail ---');
  await projectCard.first().click();
  await page.locator('input#name').waitFor({ state: 'visible', timeout: 10000 });
  const detailBody = await page.textContent('body');
  assert(detailBody.includes(projectName), 'Detail page should show project name');
  console.log('Project detail page loaded');

  // Step 4: Go to Routes tab and toggle custom routes
  console.log('\n--- Step 4: Toggle Custom Routes ---');
  const routesTab = page.locator('button[role="tab"], [data-value="routes"]').filter({ hasText: /Routes|路由/ });
  await routesTab.click();
  await page.locator('button[role="switch"]').first().waitFor({ state: 'visible', timeout: 5000 });

  // Find and click the custom routes switch
  const customRoutesSwitch = page.locator('button[role="switch"]').first();
  assert((await customRoutesSwitch.count()) > 0, 'Should find custom routes switch');

  // Toggle it on and wait for API response
  await customRoutesSwitch.click();
  await page.waitForResponse((resp) => resp.url().includes('/projects/') && resp.request().method() === 'PUT', { timeout: 5000 });
  await page.waitForTimeout(500);
  console.log('Custom routes toggled');

  // Step 5: Verify project still exists via API (the critical check)
  console.log('\n--- Step 5: Verify Project Still Exists via API ---');
  const projectsAfterToggle = await adminAPI('GET', '/projects', null, jwt);
  const foundAfterToggle = projectsAfterToggle.find((p) => p.id === project.id);
  assert(foundAfterToggle, 'Project should still appear in API list after toggling custom routes');
  assert(
    foundAfterToggle.enabledCustomRoutes && foundAfterToggle.enabledCustomRoutes.length > 0,
    'Project should have enabledCustomRoutes after toggle',
  );
  console.log(`Project still in API list, enabledCustomRoutes=${JSON.stringify(foundAfterToggle.enabledCustomRoutes)}`);

  // Step 6: Navigate back to project list and verify project is still visible
  console.log('\n--- Step 6: Navigate Back to Project List ---');
  await page.goto(`${BASE}/projects`);
  const projectCardAfter = page.locator(`text=${projectName}`);
  await projectCardAfter.first().waitFor({ state: 'visible', timeout: 10000 });
  console.log('Project still visible in list after toggle');

  // Step 7: Go back to detail, edit name via overview tab (another PUT)
  console.log('\n--- Step 7: Edit Project Name in Overview ---');
  await projectCardAfter.first().click();
  const nameInput = page.locator('input#name');
  await nameInput.waitFor({ state: 'visible', timeout: 10000 });

  const newName = `${projectName}-Edited`;
  await nameInput.fill(newName);

  // Click save button and wait for API response
  const saveButton = page.locator('button').filter({ hasText: /Save|保存/ });
  await saveButton.waitFor({ state: 'visible', timeout: 5000 });
  await saveButton.click();
  await page.waitForResponse((resp) => resp.url().includes('/projects/') && resp.request().method() === 'PUT', { timeout: 5000 });
  await page.waitForTimeout(500);
  console.log(`Project name changed to: ${newName}`);

  // Step 8: Verify project still in API list after name edit
  console.log('\n--- Step 8: Verify Project Still Exists After Name Edit ---');
  const projectsAfterEdit = await adminAPI('GET', '/projects', null, jwt);
  const foundAfterEdit = projectsAfterEdit.find((p) => p.id === project.id);
  assert(foundAfterEdit, 'Project should still appear in API list after name edit');
  assert(foundAfterEdit.name === newName, `Project name should be "${newName}", got "${foundAfterEdit?.name}"`);
  console.log('Project still in API list with updated name');

  // Step 9: Navigate to project list one more time
  console.log('\n--- Step 9: Final Project List Check ---');
  await page.goto(`${BASE}/projects`);
  const finalCard = page.locator(`text=${newName}`);
  await finalCard.first().waitFor({ state: 'visible', timeout: 10000 });
  console.log('Project visible in final list check');

  // Screenshot
  await page.screenshot({ path: '/tmp/project-update-tenant-result.png' });
  console.log('Screenshot: /tmp/project-update-tenant-result.png');

  console.log(`\n===== Test ${exitCode === 0 ? 'PASSED' : 'FAILED'} =====`);
  await cleanup();
  process.exit(exitCode);
})().catch(async (err) => {
  console.error('Test error:', err.message);
  await cleanup();
  process.exit(1);
});
