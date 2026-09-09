import { expect, test } from "playwright/test";

import { adminAPI, bodyText, loginToAdminAPI, PASS, USER } from "./helpers";

test.describe.configure({ mode: "serial" });

test.use({ video: "on" });

async function loginToAdminFrontend(page: Parameters<typeof bodyText>[0]) {
  await page.goto("/", { waitUntil: "domcontentloaded" });

  const passwordInput = page.locator('input[type="password"]').first();
  const passwordVisible = await passwordInput
    .waitFor({ state: "visible", timeout: 10000 })
    .then(() => true)
    .catch(() => false);

  if (passwordVisible) {
    const adminTab = page.getByRole("tab", { name: /Admin Login|管理员登录/i });
    if (await adminTab.isVisible().catch(() => false)) {
      await adminTab.click();
    }

    const usernameInput = page.locator('input[type="text"]').first();
    if (await usernameInput.isVisible().catch(() => false)) {
      await usernameInput.fill(USER);
    }
    await passwordInput.fill(PASS);
    await page.getByRole("button", { name: /^Login$|登录/i }).click();
  }

  await expect
    .poll(async () => /dashboard/i.test(await bodyText(page)), {
      timeout: 10000,
    })
    .toBe(true);
}

test("provider clone naming numeric policy persists through real settings and provider APIs", async ({
  page,
}, testInfo) => {
  const jwt = await loginToAdminAPI();
  const createdProviderIds: number[] = [];
  const runId = Date.now();
  const baseName = `Clone Policy OpenAI ${runId} 009`;
  const conflictName = `Clone Policy OpenAI ${runId} 010`;
  const expectedCloneName = `Clone Policy OpenAI ${runId} 011`;

  try {
    const source = await adminAPI(
      "POST",
      "/providers",
      {
        name: baseName,
        type: "custom",
        config: {
          custom: {
            baseURL: "https://mock-clone-policy.example.test/v1",
            apiKey: "mock-key-source",
          },
        },
        supportedClientTypes: ["openai"],
        supportModels: ["gpt-clone-policy-real"],
        exposedModelsEnabled: true,
        exposedModels: ["gpt-clone-policy-public"],
        maxConcurrency: 3,
      },
      jwt,
    );
    createdProviderIds.push(source.id);

    const conflict = await adminAPI(
      "POST",
      "/providers",
      {
        name: conflictName,
        type: "custom",
        config: {
          custom: {
            baseURL: "https://mock-clone-policy-conflict.example.test/v1",
            apiKey: "mock-key-conflict",
          },
        },
        supportedClientTypes: ["openai"],
      },
      jwt,
    );
    createdProviderIds.push(conflict.id);

    await loginToAdminFrontend(page);
    await page.goto("/settings", { waitUntil: "domcontentloaded" });
    await expect(page.getByText("Provider clone naming")).toBeVisible();
    await page.getByLabel("Clone naming strategy").click();
    const settingSaved = page.waitForResponse(
      (response) =>
        response
          .url()
          .includes("/api/admin/settings/provider_clone_name_strategy") &&
        response.request().method() === "PUT" &&
        response.ok(),
    );
    await page
      .getByRole("option", { name: "Increment trailing number" })
      .click();
    await settingSaved;
    await expect(
      page.getByText("Number increment advances a trailing number"),
    ).toBeVisible();
    await expect
      .poll(
        async () => {
          const value = await adminAPI(
            "GET",
            "/settings/provider_clone_name_strategy",
            undefined,
            jwt,
          );
          return value.value;
        },
        { timeout: 10000 },
      )
      .toBe("increment-number");
    await page.screenshot({
      path: testInfo.outputPath("01-real-settings-numeric-policy.png"),
      fullPage: true,
    });

    await page.goto(`/providers/${source.id}/edit`, {
      waitUntil: "domcontentloaded",
    });
    await expect(page.locator(`input[value="${baseName}"]`)).toBeVisible();

    const cloneCreated = page.waitForResponse((response) => {
      const url = response.url();
      return (
        (url.endsWith("/api/admin/providers") ||
          url.endsWith("/api/providers")) &&
        response.request().method() === "POST" &&
        response.ok()
      );
    });
    await page.getByRole("button", { name: /^Clone$/ }).click();
    const cloneResponse = await cloneCreated;
    const createdClone = (await cloneResponse.json()) as {
      id: number;
      name?: string;
    };
    createdProviderIds.push(createdClone.id);
    await expect(page).toHaveURL(
      new RegExp(`/providers/${createdClone.id}/edit`),
      {
        timeout: 10000,
      },
    );

    const providers = await adminAPI("GET", "/providers", undefined, jwt);
    const clone = providers.find(
      (item: { name?: string }) => item.name === expectedCloneName,
    );
    expect(
      clone,
      `expected clone ${expectedCloneName}; visible clone-policy providers: ${providers
        .filter((item: { name?: string }) => item.name?.includes(String(runId)))
        .map((item: { name?: string }) => item.name)
        .join(", ")}`,
    ).toBeTruthy();

    expect(clone).toMatchObject({
      name: expectedCloneName,
      type: "custom",
      supportedClientTypes: ["openai"],
      supportModels: ["gpt-clone-policy-real"],
      exposedModelsEnabled: true,
      exposedModels: ["gpt-clone-policy-public"],
      maxConcurrency: 3,
    });

    await expect(
      page.locator(`input[value="${expectedCloneName}"]`),
    ).toBeVisible();
    await page.screenshot({
      path: testInfo.outputPath("02-real-provider-clone-numeric-policy.png"),
      fullPage: true,
    });
  } finally {
    await adminAPI(
      "DELETE",
      "/settings/provider_clone_name_strategy",
      undefined,
      jwt,
    ).catch(() => undefined);
    const remaining = await adminAPI("GET", "/providers", undefined, jwt).catch(
      () => [] as Array<{ id: number; name?: string }>,
    );
    for (const item of remaining) {
      if (item.name?.includes(`Clone Policy OpenAI ${runId}`)) {
        createdProviderIds.push(item.id);
      }
    }
    for (const id of [...new Set(createdProviderIds)].reverse()) {
      await adminAPI("DELETE", `/providers/${id}`, undefined, jwt).catch(
        () => undefined,
      );
    }
  }
});
