/**
 * Playwright E2E Test: Admin Route Guard
 *
 * 测试流程：
 * 1. Admin 登录，验证可以看到「用户管理」侧边栏入口并访问 /users 页面
 * 2. 通过 API 创建 member 用户
 * 3. 登出 admin
 * 4. 以 member 身份登录，验证看不到「用户管理」侧边栏入口
 * 5. member 直接访问 /users，验证被重定向到首页
 *
 * 使用方式：
 *   先启动 maxx 服务器（需要开启 auth），然后运行：
 *   node test-admin-route-guard.mjs [base_url] [username] [password]
 *
 *   默认值：
 *     base_url = http://localhost:9880
 *     username = admin
 *     password = test123
 */
import { chromium } from 'playwright';

const BASE = process.argv[2] || 'http://localhost:9880';
const ADMIN_USER = process.argv[3] || 'admin';
const ADMIN_PASS = process.argv[4] || 'test123';
const HEADED = !!process.env.HEADED;

const MEMBER_USER = `member-e2e-${Date.now()}`;
const MEMBER_PASS = 'member-pass-123';

let exitCode = 0;
let browser = null;

function assert(condition, msg) {
  if (!condition) {
    console.error(`ASSERTION FAILED: ${msg}`);
    exitCode = 1;
    throw new Error(msg);
  }
}

// ===== Admin API Helper =====
async function adminAPI(method, apiPath, body, token) {
  const url = `${BASE}/api/admin${apiPath}`;
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
    throw new Error(`Admin API ${method} ${apiPath} failed (${res.status}): ${text}`);
  }
  return json;
}

// ===== Browser Login Helper =====
async function browserLogin(page, username, password) {
  await page.goto(BASE);
  await page.waitForSelector('input[type="text"]', { timeout: 10000 });
  await page.fill('input[type="text"]', username);
  await page.fill('input[type="password"]', password);
  await page.locator('button[type="submit"]').click();
  await page.waitForTimeout(2000);
  const bodyText = await page.textContent('body');
  assert(
    bodyText.includes('Dashboard') || bodyText.includes('dashboard'),
    `Login as ${username} should reach Dashboard`,
  );
}

// ===== Browser Logout Helper =====
async function browserLogout(page) {
  const menuBtn = page.locator('button[aria-haspopup="menu"]').last();
  await menuBtn.click();
  await page.waitForTimeout(500);

  const logoutItem = page.locator('[role="menuitem"]').filter({ hasText: /Log out|退出登录/ });
  await logoutItem.click();
  await page.waitForSelector('input[type="text"]', { timeout: 5000 });
}

// ===== Main Test =====
(async () => {
  // --- Setup: Admin login via API to create member user ---
  console.log('\n--- Setup: Create Member User via API ---');
  const loginResp = await adminAPI('POST', '/auth/login', {
    username: ADMIN_USER,
    password: ADMIN_PASS,
  });
  assert(loginResp.token, 'Should receive admin JWT token');
  const adminJwt = loginResp.token;

  const registerResp = await adminAPI(
    'POST',
    '/auth/register',
    {
      username: MEMBER_USER,
      password: MEMBER_PASS,
    },
    adminJwt,
  );
  assert(registerResp.success, 'Member user registration should succeed');
  console.log(`  Member user created: ${MEMBER_USER}`);

  // --- Browser Test ---
  console.log('\n--- Browser: Launch ---');
  browser = await chromium.launch({ headless: !HEADED });
  const context = await browser.newContext();
  const page = await context.newPage();

  // ===== Step 1: Admin login, verify Users tab visible =====
  console.log('\n--- Step 1: Admin Login - Verify Users Tab Visible ---');
  await browserLogin(page, ADMIN_USER, ADMIN_PASS);
  console.log('  Admin login success');

  const sidebar = page.locator('aside, [data-sidebar]').first();
  const usersLink = sidebar.locator('a[href="/users"]');
  const usersLinkVisible = await usersLink.isVisible().catch(() => false);
  assert(usersLinkVisible, 'Admin should see Users link in sidebar');
  console.log('  Admin can see Users link in sidebar');

  // ===== Step 2: Admin can access /users page =====
  console.log('\n--- Step 2: Admin Access /users Page ---');
  await page.goto(`${BASE}/users`);
  await page.waitForTimeout(2000);

  const adminUrl = page.url();
  assert(adminUrl.includes('/users'), `Admin should stay on /users page, got: ${adminUrl}`);

  const usersPageBody = await page.textContent('body');
  const hasUsersContent =
    usersPageBody.includes('admin') ||
    usersPageBody.includes('Users') ||
    usersPageBody.includes('用户');
  assert(hasUsersContent, 'Admin should see Users page content');
  console.log('  Admin can access /users page');

  // ===== Step 3: Logout admin =====
  console.log('\n--- Step 3: Admin Logout ---');
  await browserLogout(page);
  console.log('  Admin logged out');

  // ===== Step 4: Member login, verify Users tab NOT visible =====
  console.log('\n--- Step 4: Member Login - Verify Users Tab Hidden ---');
  await browserLogin(page, MEMBER_USER, MEMBER_PASS);
  console.log('  Member login success');

  const memberSidebar = page.locator('aside, [data-sidebar]').first();
  const memberUsersLink = memberSidebar.locator('a[href="/users"]');
  const memberUsersLinkVisible = await memberUsersLink.isVisible().catch(() => false);
  assert(!memberUsersLinkVisible, 'Member should NOT see Users link in sidebar');
  console.log('  Member cannot see Users link in sidebar');

  // ===== Step 5: Member navigates to /users, verify redirect =====
  console.log('\n--- Step 5: Member Direct Access /users - Verify Redirect ---');
  await page.goto(`${BASE}/users`);
  await page.waitForTimeout(2000);

  const memberUrl = page.url();
  const redirected = !memberUrl.includes('/users') || memberUrl.endsWith('/');
  assert(redirected, `Member should be redirected from /users, got: ${memberUrl}`);
  console.log(`  Member redirected to: ${memberUrl}`);

  const memberBody = await page.textContent('body');
  const onDashboard =
    memberBody.includes('Dashboard') ||
    memberBody.includes('dashboard') ||
    memberBody.includes('Overview') ||
    memberBody.includes('overview');
  assert(onDashboard, 'Member should be on Dashboard after redirect');
  console.log('  Member is on Dashboard after redirect from /users');

  // ===== Step 6: Member can still access other pages =====
  console.log('\n--- Step 6: Member Access Other Pages ---');
  await page.goto(`${BASE}/requests`);
  await page.waitForTimeout(2000);
  const requestsUrl = page.url();
  assert(requestsUrl.includes('/requests'), 'Member should access /requests page');
  console.log('  Member can access /requests page');

  // Screenshot
  await page.screenshot({ path: '/tmp/admin-route-guard-result.png' });
  console.log('  Screenshot: /tmp/admin-route-guard-result.png');

  console.log(`\n===== Test ${exitCode === 0 ? 'PASSED' : 'FAILED'} =====`);
  await browser.close();
  process.exit(exitCode);
})().catch(async (err) => {
  console.error('Test error:', err.message);
  if (browser) {
    try {
      await browser.close();
    } catch {}
  }
  process.exit(1);
});
