import { expect, test, type Page } from "playwright/test";

import { adminAPI, bodyText, loginToAdminAPI, PASS, USER } from "./helpers";

test.describe.configure({ mode: "serial" });

async function loginToAdminFrontend(page: Page, jwt: string) {
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
    const adminTab = page.getByRole("tab", { name: /Admin Login|管理员登录/i });
    if (await adminTab.isVisible().catch(() => false)) {
      await adminTab.click();
    }

    const usernameInput = page.locator('input[type="text"]').first();
    const usernameVisible = await usernameInput.isVisible().catch(() => false);
    if (usernameVisible) {
      await usernameInput.fill(USER);
    }
    await page.locator('input[type="password"]').first().fill(PASS);
    await page.getByRole("button", { name: /^Login$|登录/i }).click();
  }

  await expect
    .poll(async () => /dashboard/i.test(await bodyText(page)), {
      timeout: 10000,
    })
    .toBe(true);
}

async function createCustomProvider(jwt: string, name: string) {
  return adminAPI(
    "POST",
    "/providers",
    {
      name,
      type: "custom",
      config: {
        custom: {
          baseURL: `https://${name.toLowerCase().replaceAll(/[^a-z0-9]+/g, "-")}.example.test/v1`,
          apiKey: "***",
        },
      },
      supportedClientTypes: ["openai"],
      supportModels: ["preset-e2e-model"],
    },
    jwt,
  );
}

async function createMapping(
  jwt: string,
  providerID: number,
  pattern: string,
  target: string,
  priority: number,
) {
  return adminAPI(
    "POST",
    "/model-mappings",
    {
      pattern,
      target,
      scope: "provider",
      clientType: "openai",
      providerType: "custom",
      providerID,
      priority,
      isEnabled: true,
    },
    jwt,
  );
}

async function mappingsForProvider(jwt: string, providerID: number) {
  const mappings = await adminAPI("GET", "/model-mappings", undefined, jwt);
  return mappings.filter(
    (mapping: { providerID: number }) => mapping.providerID === providerID,
  );
}

async function openPresetDialog(
  page: Page,
  targetProviderName: string,
  testInfo: { outputPath: (path: string) => string },
  screenshotName: string,
) {
  await expect(page.locator("body")).toContainText(targetProviderName);
  const loadPresetButton = page.getByRole("button", {
    name: /Load preset|载入预设/i,
  });
  await loadPresetButton.scrollIntoViewIfNeeded();
  await loadPresetButton.click();
  await expect(page.getByRole("dialog")).toContainText(
    /Load existing model mappings|载入已有模型映射/i,
  );
  await page.screenshot({
    path: testInfo.outputPath(screenshotName),
    fullPage: true,
  });
}

test("provider model mappings can be manually loaded from existing provider presets", async ({
  page,
}, testInfo) => {
  const jwt = await loginToAdminAPI();
  const createdProviderIds: number[] = [];
  const runId = Date.now();
  const sourceName = `Preset Source ${runId}`;
  const duplicateSourceName = `Preset Duplicate Source ${runId}`;
  const targetName = `Preset Target ${runId}`;

  try {
    const source = await createCustomProvider(jwt, sourceName);
    const duplicateSource = await createCustomProvider(
      jwt,
      duplicateSourceName,
    );
    const target = await createCustomProvider(jwt, targetName);
    createdProviderIds.push(source.id, duplicateSource.id, target.id);

    await createMapping(
      jwt,
      source.id,
      "*sonnet*",
      "source-sonnet-model",
      1000,
    );
    await createMapping(jwt, source.id, "*haiku*", "source-haiku-model", 1010);
    await createMapping(
      jwt,
      duplicateSource.id,
      "*sonnet*",
      "source-sonnet-model",
      1000,
    );
    await createMapping(
      jwt,
      duplicateSource.id,
      "*haiku*",
      "source-haiku-model",
      1010,
    );
    await createMapping(
      jwt,
      duplicateSource.id,
      "*opus*",
      "duplicate-source-opus-model",
      1020,
    );
    await createMapping(
      jwt,
      target.id,
      "*sonnet*",
      "target-existing-sonnet",
      1000,
    );

    await page.setViewportSize({ width: 1280, height: 900 });
    await loginToAdminFrontend(page, jwt);
    await page.goto(`/providers/${target.id}/edit`);

    await openPresetDialog(
      page,
      targetName,
      testInfo,
      "01-preset-dialog-before-append.png",
    );
    await expect(page.getByRole("dialog")).not.toContainText(
      /Source provider|来源提供商/i,
    );
    await expect(page.getByRole("dialog")).toContainText("*sonnet*");
    await expect(page.getByRole("dialog")).toContainText(
      /3\/3 selected|已选 3\/3 条/i,
    );
    await page
      .getByLabel(
        /select mapping \*haiku\* to source-haiku-model|选择映射 \*haiku\* 到 source-haiku-model/i,
      )
      .setChecked(false);
    await expect(page.getByRole("dialog")).toContainText(
      /2\/3 selected|已选 2\/3 条/i,
    );
    await page.screenshot({
      path: testInfo.outputPath("01b-preset-dialog-after-deselect-haiku.png"),
      fullPage: true,
    });
    await page.getByRole("button", { name: /Apply preset|应用预设/i }).click();

    await expect
      .poll(async () => {
        const mappings = await mappingsForProvider(jwt, target.id);
        return Object.fromEntries(
          mappings.map((mapping: { pattern: string; target: string }) => [
            mapping.pattern,
            mapping.target,
          ]),
        );
      })
      .toEqual({
        "*sonnet*": "target-existing-sonnet",
        "*opus*": "duplicate-source-opus-model",
      });
    await page.screenshot({
      path: testInfo.outputPath("02-after-append-missing-only.png"),
      fullPage: true,
    });

    await openPresetDialog(
      page,
      targetName,
      testInfo,
      "03-preset-dialog-before-overwrite.png",
    );
    await page.getByText(/Overwrite conflicts|覆盖冲突项/i).click();
    await page.getByRole("button", { name: /Apply preset|应用预设/i }).click();
    await expect
      .poll(async () => {
        const mappings = await mappingsForProvider(jwt, target.id);
        return Object.fromEntries(
          mappings.map((mapping: { pattern: string; target: string }) => [
            mapping.pattern,
            mapping.target,
          ]),
        );
      })
      .toMatchObject({
        "*sonnet*": "source-sonnet-model",
        "*haiku*": "source-haiku-model",
        "*opus*": "duplicate-source-opus-model",
      });
    await page.screenshot({
      path: testInfo.outputPath("04-after-overwrite-conflict.png"),
      fullPage: true,
    });

    await createMapping(jwt, target.id, "*extra*", "target-extra-model", 1990);
    await page.reload();
    await openPresetDialog(
      page,
      targetName,
      testInfo,
      "05-preset-dialog-before-replace.png",
    );
    await page
      .getByText(/Replace all current mappings|替换全部当前映射/i)
      .click();
    await page.getByRole("button", { name: /Apply preset|应用预设/i }).click();

    await expect
      .poll(async () => {
        const mappings = await mappingsForProvider(jwt, target.id);
        return mappings
          .map((mapping: { pattern: string; target: string }) => [
            mapping.pattern,
            mapping.target,
          ])
          .sort();
      })
      .toEqual([
        ["*haiku*", "source-haiku-model"],
        ["*opus*", "duplicate-source-opus-model"],
        ["*sonnet*", "source-sonnet-model"],
      ]);
    await page.screenshot({
      path: testInfo.outputPath("06-after-replace-all.png"),
      fullPage: true,
    });
  } finally {
    for (const providerID of createdProviderIds) {
      const mappings = await mappingsForProvider(jwt, providerID).catch(
        () => [],
      );
      for (const mapping of mappings) {
        await adminAPI(
          "DELETE",
          `/model-mappings/${mapping.id}`,
          undefined,
          jwt,
        ).catch(() => undefined);
      }
    }
    for (const providerID of createdProviderIds.reverse()) {
      await adminAPI(
        "DELETE",
        `/providers/${providerID}`,
        undefined,
        jwt,
      ).catch(() => undefined);
    }
  }
});
