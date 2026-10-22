import { expect, test, type Page } from "playwright/test";

const KEYS = ["sk-visibility-first", "sk-visibility-second"];

async function mockProvider(
  page: Page,
  type: string,
  excludeFromExport = false,
) {
  const writes: Record<string, any>[] = [];
  const provider = {
    id: 42,
    name: "Key visibility test",
    type,
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
    supportedClientTypes: ["openai"],
    supportModels: [],
    excludeFromExport,
    config: {
      custom: excludeFromExport
        ? {}
        : {
            baseURL: "https://upstream.example.test",
            apiKey: KEYS[0],
            apiKeys: KEYS,
          },
    },
  };
  await page.addInitScript(() =>
    localStorage.setItem("maxx-ui-language", "en"),
  );
  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname.replace(
      /^\/api(?:\/admin)?/,
      "",
    );
    let body: unknown = [];
    if (path === "/auth/status") body = { authEnabled: false };
    else if (path === "/settings") body = { ui_multitenant_enabled: "false" };
    else if (path === "/providers" || path === "/providers/42") {
      if (request.method() === "POST" || request.method() === "PUT") {
        const payload = request.postDataJSON();
        writes.push(payload);
        body = { ...provider, ...payload };
      } else body = path === "/providers" ? [provider] : provider;
    } else if (path.includes("runtime-models"))
      body = { available: false, models: [] };
    else if (path === "/proxy-status")
      body = { running: true, address: "127.0.0.1", port: 9880 };
    else if (path.endsWith("/counts")) body = {};
    await route.fulfill({
      contentType: "application/json",
      body: JSON.stringify(body),
    });
  });
  return writes;
}

for (const type of ["custom", "newapi"]) {
  for (const flow of ["create", "edit"]) {
    test(`${type} ${flow} masks the key pool and toggles without changing saved keys`, async ({
      page,
    }, testInfo) => {
      const writes = await mockProvider(page, type);
      await page.goto(
        flow === "create" ? `/providers/create/${type}` : "/providers/42/edit",
      );
      const field = page.locator("textarea").first();
      if (flow === "create") {
        await page.locator("main input").first().fill("Key visibility test");
        await page
          .locator('input[placeholder*="https://"]')
          .first()
          .fill("https://upstream.example.test");
        await field.fill(KEYS.join("\n"));
      }
      await expect(field).toHaveValue(KEYS.join("\n"));
      await expect(field).toHaveCSS("-webkit-text-security", "disc");
      await field.screenshot({ path: testInfo.outputPath("keys-hidden.png") });
      const toggle = field.locator("..").getByRole("button");
      await toggle.click();
      await expect(field).toHaveCSS("-webkit-text-security", "none");
      await expect(field).toHaveValue(KEYS.join("\n"));
      await field.screenshot({ path: testInfo.outputPath("keys-visible.png") });
      await toggle.click();
      await expect(field).toHaveCSS("-webkit-text-security", "disc");
      const updatedKeys = [...KEYS, "sk-visibility-third"];
      await field.fill(updatedKeys.join("\n"));
      await expect(field).toHaveCSS("-webkit-text-security", "disc");
      await page
        .getByRole("button", {
          name: flow === "create" ? "Create Provider" : "Save Changes",
          exact: true,
        })
        .click();
      await expect.poll(() => writes.length).toBe(1);
      expect(writes[0].config.custom.apiKeys).toEqual(updatedKeys);
    });
  }

  test(`${type} export-excluded replacement keys stay masked without a reveal button`, async ({
    page,
  }) => {
    await mockProvider(page, type, true);
    await page.goto("/providers/42/edit");
    const field = page.locator("textarea").first();
    await expect(field).toHaveValue("");
    await field.fill(KEYS.join("\n"));
    await expect(field).toHaveCSS("-webkit-text-security", "disc");
    await expect(field.locator("..").getByRole("button")).toHaveCount(0);
  });
}
