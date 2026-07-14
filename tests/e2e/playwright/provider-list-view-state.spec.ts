import { expect, test } from "playwright/test";

import { adminAPI, bodyText, loginToAdminAPI } from "./helpers";

test.describe.configure({ mode: "serial" });

async function loginToAdminFrontend(
  page: Parameters<typeof bodyText>[0],
  jwt: string,
) {
  await adminAPI(
    "PUT",
    "/settings/ui_multitenant_enabled",
    { value: "true" },
    jwt,
  );
  await page.goto("/");

  const passwordInput = page.locator('input[type="password"]');
  const passwordVisible = await passwordInput
    .waitFor({ state: "visible", timeout: 10000 })
    .then(() => true)
    .catch(() => false);

  if (passwordVisible) {
    const usernameInput = page.locator('input[type="text"]');
    const usernameVisible = await usernameInput.isVisible().catch(() => false);
    if (usernameVisible) {
      await usernameInput.fill("admin");
    }
    await passwordInput.fill("test123");
    await page.locator('button[type="submit"]').click();
  }

  await expect
    .poll(async () => /dashboard/i.test(await bodyText(page)))
    .toBe(true);
}

test("providers list restores search filter and scroll position after editing a provider", async ({
  page,
}, testInfo) => {
  const jwt = await loginToAdminAPI();
  const createdProviderIds: number[] = [];
  const runId = Date.now();
  const providerPrefix = `Restore State Provider ${runId}`;

  try {
    for (let index = 0; index < 24; index += 1) {
      const provider = await adminAPI(
        "POST",
        "/providers",
        {
          name: `${providerPrefix} ${String(index).padStart(2, "0")}`,
          type: "custom",
          config: {
            custom: {
              baseURL: `https://mock-provider-${index}.example.test/v1`,
              apiKey: `sk-restore-state-${index}`,
            },
          },
          supportedClientTypes: ["openai"],
          supportModels: [`restore-state-model-${index}`],
        },
        jwt,
      );
      createdProviderIds.push(provider.id);
    }

    const targetProviderId = createdProviderIds[18];
    const targetProviderName = `${providerPrefix} 18`;

    await page.setViewportSize({ width: 1280, height: 720 });
    await loginToAdminFrontend(page, jwt);
    await page.goto("/providers");

    const searchInput = page.getByPlaceholder(/search|搜索/i);
    await searchInput.fill(providerPrefix);
    await expect
      .poll(() => new URL(page.url()).searchParams.get("q"))
      .toBe(providerPrefix);

    const listScroller = page.locator(".flex-1.overflow-y-auto").first();
    const targetRow = page.locator(`[data-provider-id="${targetProviderId}"]`);
    await expect(targetRow).toContainText(targetProviderName);
    await targetRow.evaluate((element) =>
      element.scrollIntoView({ block: "center" }),
    );

    const beforeScrollTop = await listScroller.evaluate(
      (element) => element.scrollTop,
    );
    expect(beforeScrollTop).toBeGreaterThan(0);
    await expect(targetRow).toBeVisible();
    await page.screenshot({
      path: testInfo.outputPath("01-filtered-provider-list-before-edit.png"),
      fullPage: true,
    });

    await targetRow.click();
    await expect(page).toHaveURL(
      new RegExp(`/providers/${targetProviderId}/edit`),
    );
    await expect(page.locator("body")).toContainText(targetProviderName);
    await page.screenshot({
      path: testInfo.outputPath("02-provider-edit-page.png"),
      fullPage: true,
    });

    await page.goBack();

    await expect(page).toHaveURL(/\/providers/);
    await expect
      .poll(() => new URL(page.url()).searchParams.get("q"))
      .toBe(providerPrefix);
    await expect(searchInput).toHaveValue(providerPrefix);
    await expect(targetRow).toBeVisible();

    const afterScrollTop = await listScroller.evaluate(
      (element) => element.scrollTop,
    );
    expect(afterScrollTop).toBeGreaterThan(0);
    expect(Math.abs(afterScrollTop - beforeScrollTop)).toBeLessThan(260);
    await page.screenshot({
      path: testInfo.outputPath("03-filtered-provider-list-after-return.png"),
      fullPage: true,
    });
  } finally {
    for (const providerId of createdProviderIds.reverse()) {
      await adminAPI(
        "DELETE",
        `/providers/${providerId}`,
        undefined,
        jwt,
      ).catch(() => undefined);
    }
  }
});
