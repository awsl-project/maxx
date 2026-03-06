/**
 * Playwright 虚拟认证器 Passkey Discoverable Login 测试
 *
 * 测试流程：
 * 1. 密码登录
 * 2. 注册 Passkey（resident key）
 * 3. 登出
 * 4. 不输入用户名，直接点 "Login with Passkey" 完成 discoverable 登录
 *
 * 使用方式：
 *   先启动 maxx 服务器（需要开启 auth），然后运行：
 *   node test-passkey-discoverable.mjs [base_url] [username] [password]
 *
 *   默认值：
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

function assert(condition, msg) {
  if (!condition) {
    console.error(`❌ ASSERTION FAILED: ${msg}`);
    exitCode = 1;
    throw new Error(msg);
  }
}

(async () => {
  const browser = await chromium.launch({ headless: !HEADED });
  const context = await browser.newContext();
  const page = await context.newPage();

  // 启用 CDP 虚拟认证器（resident key + user verification）
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
  console.log(`   Target: ${BASE}, User: ${USER}`);

  // ===== Step 1: 密码登录 =====
  console.log('\n--- Step 1: Password Login ---');
  await page.goto(BASE);
  // 等待登录页加载
  await page.waitForSelector('input[type="text"]', { timeout: 10000 });
  await page.fill('input[type="text"]', USER);
  await page.fill('input[type="password"]', PASS);
  // 点击主登录按钮（排除 Passkey 按钮）
  const loginBtn = page.locator('button[type="submit"]');
  await loginBtn.click();
  await page.waitForTimeout(2000);
  // 检查是否登录成功（看到 Dashboard 或管理界面）
  const bodyText = await page.textContent('body');
  assert(bodyText.includes('Dashboard') || bodyText.includes('dashboard'), 'Password login should reach Dashboard');
  console.log('✅ Password login success');

  // ===== Step 2: 注册 Passkey =====
  console.log('\n--- Step 2: Register Passkey ---');
  // 打开用户菜单
  const menuBtn = page.locator('button[aria-haspopup="menu"]').last();
  await menuBtn.click();
  await page.waitForTimeout(500);

  // 查找并点击 Manage Passkeys 菜单
  const passkeyMenuItem = page.locator('[role="menuitem"]').filter({ hasText: /Passkey|passkey/ });
  await passkeyMenuItem.click();
  await page.waitForTimeout(1000);
  console.log('  Passkey Management dialog opened');

  // 点击注册按钮
  const registerBtn = page.locator('button').filter({ hasText: /Register Passkey|注册 Passkey/ });
  await registerBtn.click();

  // 等待注册完成（虚拟认证器自动完成）
  await page.waitForTimeout(3000);

  // 验证：应该能看到 "Passkey 1" 或类似的已注册条目
  const dialogText = await page.locator('[role="dialog"]').textContent();
  const registered = dialogText.includes('Passkey 1') || dialogText.includes('passkey');
  console.log(`  Dialog content preview: ${dialogText.substring(0, 200)}`);
  assert(registered, 'Passkey should be registered and visible in the list');
  console.log('✅ Passkey registered successfully');

  // 验证虚拟认证器中有 credential
  const { credentials } = await cdp.send('WebAuthn.getCredentials', { authenticatorId });
  console.log(`  Virtual authenticator credentials: ${credentials.length}`);
  assert(credentials.length > 0, 'Virtual authenticator should have at least 1 credential');
  // 验证是 resident credential
  assert(credentials[0].isResidentCredential, 'Credential should be a resident credential (discoverable)');
  console.log('✅ Credential is resident (discoverable)');

  // 关闭对话框（可能有多个 Close 按钮，用 first()）
  const closeBtn = page.locator('[role="dialog"] button').filter({ hasText: /Close|关闭/ }).first();
  if (await closeBtn.isVisible()) {
    await closeBtn.click();
    await page.waitForTimeout(500);
  }

  // ===== Step 3: 登出 =====
  console.log('\n--- Step 3: Logout ---');
  const menuBtn2 = page.locator('button[aria-haspopup="menu"]').last();
  await menuBtn2.click();
  await page.waitForTimeout(500);

  const logoutItem = page.locator('[role="menuitem"]').filter({ hasText: /Log out|退出登录/ });
  await logoutItem.click();
  await page.waitForSelector('input[type="text"]', { timeout: 5000 });
  console.log('✅ Logged out');

  // ===== Step 4: Discoverable Passkey 登录（不输入用户名） =====
  console.log('\n--- Step 4: Discoverable Passkey Login (NO username) ---');

  // 确保用户名为空
  const usernameField = page.locator('input[type="text"]');
  await usernameField.fill('');
  const usernameValue = await usernameField.inputValue();
  assert(usernameValue === '', 'Username field should be empty');
  console.log('  Username field is empty: ✓');

  // 点击 Login with Passkey
  const passkeyLoginBtn = page.locator('button').filter({ hasText: /Login with Passkey|使用 Passkey 登录/ });
  // 验证按钮是可点击的（之前有 disabled 守卫，现在应该没有了）
  const isDisabled = await passkeyLoginBtn.isDisabled();
  assert(!isDisabled, 'Passkey login button should NOT be disabled when username is empty');
  console.log('  Passkey login button is enabled without username: ✓');

  await passkeyLoginBtn.click();

  // 虚拟认证器自动用 resident key 完成
  try {
    await page.waitForTimeout(3000);
    const bodyText2 = await page.textContent('body');
    if (bodyText2.includes('Dashboard') || bodyText2.includes('dashboard')) {
      console.log('✅ Discoverable passkey login SUCCESS!');
    } else {
      // 可能有错误信息
      const errorText = await page.locator('.text-destructive').textContent().catch(() => '');
      console.log(`❌ Discoverable passkey login failed. Error: ${errorText}`);
      console.log(`  Page text: ${bodyText2.substring(0, 300)}`);
      exitCode = 1;
    }
  } catch (e) {
    console.log(`❌ Discoverable passkey login error: ${e.message}`);
    exitCode = 1;
  }

  // ===== Step 5: 验证登录后状态 =====
  console.log('\n--- Step 5: Verify logged-in state ---');
  const finalBody = await page.textContent('body');
  if (finalBody.includes('Dashboard') || finalBody.includes('dashboard')) {
    console.log('✅ Dashboard visible - discoverable passkey login confirmed');
  } else {
    console.log('❌ Dashboard not visible after login');
    exitCode = 1;
  }

  // 截图
  await page.screenshot({ path: '/tmp/passkey-discoverable-result.png' });
  console.log('  Screenshot: /tmp/passkey-discoverable-result.png');

  console.log(`\n===== Test ${exitCode === 0 ? 'PASSED' : 'FAILED'} =====`);
  await browser.close();
  process.exit(exitCode);
})().catch(async (err) => {
  console.error('❌ Test error:', err.message);
  process.exit(1);
});
