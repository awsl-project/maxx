/**
 * Playwright 虚拟认证器 Passkey Discoverable Login 测试
 *
 * 测试流程：
 * Part A — Admin 用户：密码登录 → 注册 passkey → 登出 → discoverable 登录 → 验证用户信息
 * Part B — 新用户：注册 → admin 审批 → 密码登录 → 注册 passkey → 登出 → discoverable 登录 → 验证用户信息
 *
 * 使用方式：
 *   先启动 maxx 服务器（需要开启 auth），然后运行：
 *   node test-passkey-discoverable.mjs [base_url] [admin_user] [admin_pass]
 */
import { chromium } from 'playwright';

const BASE = process.argv[2] || 'http://localhost:9880';
const ADMIN_USER = process.argv[3] || 'admin';
const ADMIN_PASS = process.argv[4] || 'test123';
const NEW_USER = 'testuser_' + Date.now().toString(36);
const NEW_PASS = 'newpass123';
const HEADED = !!process.env.HEADED;

let exitCode = 0;

function assert(condition, msg) {
  if (!condition) {
    console.error(`❌ ASSERTION FAILED: ${msg}`);
    exitCode = 1;
    throw new Error(msg);
  }
}

// ===== Helper: 密码登录 =====
async function passwordLogin(page, username, password) {
  await page.goto(BASE);
  await page.waitForSelector('input[type="text"]', { timeout: 10000 });
  await page.fill('input[type="text"]', username);
  await page.fill('input[type="password"]', password);
  await page.locator('button[type="submit"]').click();
  await page.waitForTimeout(2000);
  const body = await page.textContent('body');
  assert(body.includes('Dashboard') || body.includes('dashboard'), `Password login failed for ${username}`);
  console.log(`✅ Password login success: ${username}`);
}

// ===== Helper: 注册 Passkey =====
async function registerPasskey(page, cdp, authenticatorId) {
  const menuBtn = page.locator('button[aria-haspopup="menu"]').last();
  await menuBtn.click();
  await page.waitForTimeout(500);
  await page.locator('[role="menuitem"]').filter({ hasText: /Passkey|passkey/ }).click();
  await page.waitForTimeout(1000);
  console.log('  Passkey Management dialog opened');

  await page.locator('button').filter({ hasText: /Register Passkey|注册 Passkey/ }).click();
  await page.waitForTimeout(3000);

  const dialogText = await page.locator('[role="dialog"]').textContent();
  assert(dialogText.includes('Passkey 1') || dialogText.includes('passkey'), 'Passkey should be registered');
  console.log('✅ Passkey registered');

  const { credentials } = await cdp.send('WebAuthn.getCredentials', { authenticatorId });
  assert(credentials.length > 0, 'Should have at least 1 credential');
  assert(credentials[credentials.length - 1].isResidentCredential, 'Credential should be resident (discoverable)');
  console.log(`✅ Credential is resident (total: ${credentials.length})`);

  await page.locator('[role="dialog"] button').filter({ hasText: /Close|关闭/ }).first().click();
  await page.waitForTimeout(500);
}

// ===== Helper: 登出 =====
async function logout(page) {
  const menuBtn = page.locator('button[aria-haspopup="menu"]').last();
  await menuBtn.click();
  await page.waitForTimeout(500);
  await page.locator('[role="menuitem"]').filter({ hasText: /Log out|退出登录/ }).click();
  await page.waitForSelector('input[type="text"]', { timeout: 5000 });
  console.log('✅ Logged out');
}

// ===== Helper: Discoverable Passkey 登录 =====
async function discoverablePasskeyLogin(page) {
  const usernameField = page.locator('input[type="text"]');
  await usernameField.fill('');
  assert(await usernameField.inputValue() === '', 'Username field should be empty');
  console.log('  Username field is empty: ✓');

  const passkeyBtn = page.locator('button').filter({ hasText: /Login with Passkey|使用 Passkey 登录/ });
  assert(!(await passkeyBtn.isDisabled()), 'Passkey button should be enabled without username');
  console.log('  Passkey login button enabled: ✓');

  await passkeyBtn.click();
  await page.waitForTimeout(3000);

  const body = await page.textContent('body');
  assert(body.includes('Dashboard') || body.includes('dashboard'), 'Discoverable passkey login should reach Dashboard');
  console.log('✅ Discoverable passkey login SUCCESS!');
}

// ===== Helper: 验证登录后用户信息 =====
async function verifyUserInfo(page, expectedUsername) {
  await page.waitForTimeout(1000);

  // 右上角 Account badge
  const accountBadge = page.locator('header').locator('text=/Account|账户/').first();
  const accountVisible = await accountBadge.isVisible().catch(() => false);
  if (accountVisible) {
    const accountText = await accountBadge.locator('..').textContent();
    assert(accountText.includes(expectedUsername), `Account badge should contain "${expectedUsername}", got: "${accountText.trim()}"`);
    console.log(`✅ Top-right: "${accountText.trim()}"`);
  } else {
    console.log('❌ Top-right account badge NOT visible');
    exitCode = 1;
  }

  // 左下角 sidebar 用户名
  const sidebarUserSpan = page.locator('nav span.truncate.text-xs.font-medium').first();
  const sidebarVisible = await sidebarUserSpan.isVisible().catch(() => false);
  if (sidebarVisible) {
    const sidebarText = await sidebarUserSpan.textContent();
    assert(sidebarText.trim().toLowerCase().includes(expectedUsername.toLowerCase()),
      `Sidebar should show "${expectedUsername}", got: "${sidebarText.trim()}"`);
    console.log(`✅ Sidebar: "${sidebarText.trim()}"`);
  } else {
    // Collapsed — check via menu
    await page.locator('button[title="Menu"]').last().click();
    await page.waitForTimeout(500);
    const menuText = await page.locator('[role="menuitem"], [data-slot="dropdown-menu-label"]').first().textContent().catch(() => '');
    assert(menuText.toLowerCase().includes(expectedUsername.toLowerCase()),
      `Menu should show "${expectedUsername}", got: "${menuText.trim()}"`);
    console.log(`✅ Sidebar menu: "${menuText.trim()}"`);
    await page.keyboard.press('Escape');
  }
}

// ===== Main =====
(async () => {
  const browser = await chromium.launch({ headless: !HEADED });
  const context = await browser.newContext();
  const page = await context.newPage();

  // 启用 CDP 虚拟认证器
  const cdp = await context.newCDPSession(page);
  await cdp.send('WebAuthn.enable');
  const { authenticatorId } = await cdp.send('WebAuthn.addVirtualAuthenticator', {
    options: {
      protocol: 'ctap2',
      transport: 'internal',
      hasResidentKey: true,
      hasUserVerification: true,
      isUserVerified: true,
    },
  });
  console.log(`✅ Virtual authenticator added: ${authenticatorId}`);
  console.log(`   Target: ${BASE}`);
  console.log(`   Admin: ${ADMIN_USER}, New user: ${NEW_USER}`);

  // ============================================
  // Part A: Admin 用户测试
  // ============================================
  console.log('\n========== Part A: Admin User ==========');

  console.log('\n--- A1: Admin password login ---');
  await passwordLogin(page, ADMIN_USER, ADMIN_PASS);

  console.log('\n--- A2: Admin register passkey ---');
  await registerPasskey(page, cdp, authenticatorId);

  console.log('\n--- A3: Admin logout ---');
  await logout(page);

  console.log('\n--- A4: Admin discoverable passkey login ---');
  await discoverablePasskeyLogin(page);

  console.log('\n--- A5: Verify admin user info ---');
  await verifyUserInfo(page, ADMIN_USER);
  await page.screenshot({ path: '/tmp/passkey-admin-result.png' });
  console.log('  Screenshot: /tmp/passkey-admin-result.png');

  // 获取 admin token 用于后续审批新用户
  const adminToken = await page.evaluate(() => localStorage.getItem('maxx-admin-token'));
  assert(adminToken, 'Admin token should exist in localStorage');

  // ============================================
  // Part B: 新用户测试
  // ============================================
  console.log('\n========== Part B: New User ==========');

  // B1: 注册新用户（通过前端注册表单）
  console.log('\n--- B1: Register new user ---');
  await logout(page);

  // 点击 Register 按钮切换到注册模式
  await page.locator('button').filter({ hasText: /Register|注册/ }).last().click();
  await page.waitForTimeout(500);

  await page.fill('input[type="text"]', NEW_USER);
  const passwordFields = page.locator('input[type="password"]');
  await passwordFields.nth(0).fill(NEW_PASS);
  await passwordFields.nth(1).fill(NEW_PASS);
  await page.locator('button[type="submit"]').click();
  await page.waitForTimeout(2000);

  // 检查注册成功消息或自动回到登录页
  const pageText = await page.textContent('body');
  const registerOk = pageText.includes('pending') || pageText.includes('success') ||
    pageText.includes('审核') || pageText.includes('Login');
  assert(registerOk, 'New user registration should succeed');
  console.log(`✅ New user "${NEW_USER}" registered (pending approval)`);

  // B2: Admin 审批新用户（通过 API）
  console.log('\n--- B2: Admin approves new user ---');
  // 先获取用户列表找到新用户 ID
  const listResp = await fetch(`${BASE}/api/admin/users`, {
    headers: { 'Authorization': `Bearer ${adminToken}` },
  });
  const users = await listResp.json();
  const newUserObj = users.find(u => u.username === NEW_USER);
  assert(newUserObj, `User "${NEW_USER}" should exist in user list`);
  console.log(`  Found new user ID: ${newUserObj.id}, status: ${newUserObj.status}`);

  const approveResp = await fetch(`${BASE}/api/admin/users/${newUserObj.id}/approve`, {
    method: 'PUT',
    headers: { 'Authorization': `Bearer ${adminToken}` },
  });
  assert(approveResp.ok, `Approve should succeed, got ${approveResp.status}`);
  console.log(`✅ User "${NEW_USER}" approved`);

  // B3: 新用户密码登录
  console.log('\n--- B3: New user password login ---');
  await page.goto(BASE);
  await page.waitForTimeout(500);
  await passwordLogin(page, NEW_USER, NEW_PASS);

  // B4: 新用户注册 Passkey
  console.log('\n--- B4: New user register passkey ---');
  await registerPasskey(page, cdp, authenticatorId);

  // B5: 新用户登出
  console.log('\n--- B5: New user logout ---');
  await logout(page);

  // B6: 新用户 discoverable passkey 登录（不输入用户名）
  // 注意：虚拟认证器现在有 2 个 resident key（admin + newuser）
  // discoverable login 会让认证器选择，CDP 虚拟认证器会自动选最后一个注册的
  console.log('\n--- B6: New user discoverable passkey login ---');
  await discoverablePasskeyLogin(page);

  // B7: 验证新用户信息
  console.log('\n--- B7: Verify new user info ---');
  await verifyUserInfo(page, NEW_USER);
  await page.screenshot({ path: '/tmp/passkey-newuser-result.png' });
  console.log('  Screenshot: /tmp/passkey-newuser-result.png');

  console.log(`\n===== Test ${exitCode === 0 ? 'PASSED' : 'FAILED'} =====`);
  await browser.close();
  process.exit(exitCode);
})().catch(async (err) => {
  console.error('❌ Test error:', err.message);
  process.exit(1);
});
