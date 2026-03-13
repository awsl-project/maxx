import { expect, type Page } from 'playwright/test';

export const BASE = process.env.MAXX_E2E_BASE_URL || 'http://localhost:9880';
export const USER = process.env.MAXX_E2E_USERNAME || 'admin';
export const PASS = process.env.MAXX_E2E_PASSWORD || 'test123';

export function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

export async function adminAPI(method: string, path: string, body?: unknown, token?: string): Promise<any> {
  const url = `${BASE}/api/admin${path}`;
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
  };

  if (token) {
    headers.Authorization = `Bearer ${token}`;
  }

  const response = await fetch(url, {
    method,
    headers,
    body: body ? JSON.stringify(body) : undefined,
  });

  const text = await response.text();
  let json: any = text;
  try {
    json = JSON.parse(text);
  } catch {
    // keep text payload for error reporting
  }

  if (!response.ok) {
    throw new Error(`Admin API ${method} ${path} failed (${response.status}): ${text}`);
  }

  return json;
}

export async function bodyText(page: Page): Promise<string> {
  return (await page.textContent('body')) ?? '';
}

export async function waitForDashboard(page: Page, timeout = 10000) {
  await expect.poll(async () => /dashboard/i.test(await bodyText(page)), { timeout }).toBe(true);
}

export async function loginToAdminUI(page: Page) {
  await page.goto(BASE);
  await expect(page.locator('input[type="text"]')).toBeVisible({ timeout: 10000 });
  await page.fill('input[type="text"]', USER);
  await page.fill('input[type="password"]', PASS);
  await page.locator('button[type="submit"]').click();
  await waitForDashboard(page);
}

export async function loginToAdminAPI(): Promise<string> {
  const loginResponse = await adminAPI('POST', '/auth/login', {
    username: USER,
    password: PASS,
  });
  expect(loginResponse.token).toBeTruthy();
  return loginResponse.token;
}

export async function nextAnimationFrames(page: Page, count = 1) {
  for (let i = 0; i < count; i += 1) {
    await page.evaluate(
      () =>
        new Promise<void>((resolve) => {
          requestAnimationFrame(() => resolve());
        }),
    );
  }
}

export async function closeServer(server: { close: (callback: (error?: Error | undefined) => void) => void }) {
  await new Promise<void>((resolve, reject) => {
    server.close((error?: Error) => {
      if (error) {
        reject(error);
        return;
      }
      resolve();
    });
  });
}
