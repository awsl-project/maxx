import { expect, test, type Page } from "playwright/test";

const providers = [
  {
    id: 101,
    createdAt: "2026-06-27T00:00:00Z",
    updatedAt: "2026-06-27T00:00:00Z",
    type: "custom",
    name: "Claude Expired A",
    config: {
      custom: { baseURL: "https://expired-a.example.com", apiKey: "sk-a" },
    },
    supportedClientTypes: ["claude"],
    supportModels: ["claude-*"],
  },
  {
    id: 102,
    createdAt: "2026-06-27T00:00:00Z",
    updatedAt: "2026-06-27T00:00:00Z",
    type: "custom",
    name: "Claude Expired B",
    config: {
      custom: { baseURL: "https://expired-b.example.com", apiKey: "sk-b" },
    },
    supportedClientTypes: ["claude"],
    supportModels: ["claude-*"],
  },
];

const routes = [
  {
    id: 201,
    createdAt: "2026-06-27T00:00:00Z",
    updatedAt: "2026-06-27T00:00:00Z",
    isEnabled: true,
    isNative: true,
    projectID: 0,
    clientType: "claude",
    providerID: 101,
    position: 1,
    weight: 1,
  },
  {
    id: 202,
    createdAt: "2026-06-27T00:00:00Z",
    updatedAt: "2026-06-27T00:00:00Z",
    isEnabled: true,
    isNative: true,
    projectID: 0,
    clientType: "claude",
    providerID: 102,
    position: 2,
    weight: 1,
  },
];

async function mockRouteApis(page: Page, bulkDeleteBodies: unknown[]) {
  await page.route("**/api/**", async (route) => {
    const url = new URL(route.request().url());
    const { pathname } = url;
    const method = route.request().method();

    const json = (body: unknown, status = 200) =>
      route.fulfill({
        status,
        contentType: "application/json",
        body: JSON.stringify(body),
      });

    if (pathname === "/api/admin/auth/status") {
      return json({ authEnabled: false });
    }

    if (pathname === "/api/settings" || pathname === "/api/admin/settings") {
      return json({ ui_multitenant_enabled: "false" });
    }

    if (pathname === "/api/providers" || pathname === "/api/admin/providers") {
      return json(providers);
    }

    if (pathname === "/api/routes" || pathname === "/api/admin/routes") {
      return json(routes);
    }

    if (
      pathname === "/api/routes/claude-provider-batch-test" &&
      method === "POST"
    ) {
      return json({
        clientType: "claude",
        projectID: 0,
        testModel: "claude-sonnet-4",
        persistMode: "passed",
        createRoutes: true,
        concurrency: 2,
        testedCount: 2,
        usableCount: 0,
        persistedCount: 0,
        routesCreated: 0,
        routesUpdated: 0,
        routesDisabled: 0,
        routesSkipped: 0,
        results: providers.map((provider, index) => ({
          index,
          source: "existing",
          existingID: provider.id,
          providerID: provider.id,
          name: provider.name,
          type: provider.type,
          baseURL: provider.config.custom.baseURL,
          requestedModel: "claude-sonnet-4",
          mappedModel: "claude-sonnet-4",
          action: "test_existing_route",
          status: "auth_failed",
          httpStatus: 401,
          ok: false,
          persisted: false,
          routeCreated: false,
          routeUpdated: false,
          routeEnabled: true,
          message: "expired token",
          error: "expired token",
          durationMs: 42,
        })),
      });
    }

    if (pathname === "/api/providers/bulk-delete" && method === "POST") {
      const payload = route.request().postDataJSON();
      bulkDeleteBodies.push(payload);
      return json({
        deletedCount: 2,
        deletedIDs: [101, 102],
        notFoundIDs: [],
        routeDeletedCount: 2,
        modelMappingDeletedCount: 2,
      });
    }

    if (
      pathname === "/api/projects" ||
      pathname === "/api/admin/projects" ||
      pathname === "/api/admin/routing-strategies" ||
      pathname === "/api/requests/active" ||
      pathname === "/api/admin/requests/active"
    ) {
      return json([]);
    }

    if (
      pathname === "/api/provider-stats" ||
      pathname === "/api/admin/provider-stats"
    ) {
      return json({});
    }

    if (
      pathname === "/api/model-mappings" ||
      pathname === "/api/admin/model-mappings"
    ) {
      return json([]);
    }

    return route.fulfill({
      status: 404,
      contentType: "application/json",
      body: JSON.stringify({ error: "Unmocked endpoint", pathname, method }),
    });
  });
}

test("claude batch test removes failed existing providers with one bulk request", async ({
  page,
}, testInfo) => {
  const bulkDeleteBodies: unknown[] = [];
  await mockRouteApis(page, bulkDeleteBodies);

  page.on("dialog", (dialog) => dialog.accept());

  await page.goto("/routes/claude");

  await expect(
    page.getByRole("heading", { name: /Claude Routes|Claude 路由/ }),
  ).toBeVisible({
    timeout: 10000,
  });
  await page
    .getByRole("button", { name: /Batch test providers|批量测试提供商/ })
    .click();
  await expect(
    page.getByRole("heading", {
      name: /Claude provider batch test|Claude 提供商批量测试/,
    }),
  ).toBeVisible();

  await page.getByRole("button", { name: /Select all|全选|选择全部/ }).click();
  await page
    .getByRole("button", { name: /Start concurrent test|开始并发测试/ })
    .click();

  const dialog = page.getByRole("dialog", {
    name: /Claude provider batch test|Claude 提供商批量测试/,
  });
  await expect(
    dialog.getByText(
      /Existing providers pending removal confirmation|待确认移除的已有提供商/,
    ),
  ).toBeVisible();
  await expect(dialog.getByText("Claude Expired A").first()).toBeVisible();
  await expect(dialog.getByText("Claude Expired B").first()).toBeVisible();

  const beforeScreenshotPath = testInfo.outputPath(
    "claude-batch-test-failed-existing-before-bulk-remove.png",
  );
  await page.screenshot({ path: beforeScreenshotPath, fullPage: true });
  await testInfo.attach("failed existing providers before bulk remove", {
    path: beforeScreenshotPath,
    contentType: "image/png",
  });

  await page
    .getByRole("button", { name: /Select all failed items|选择全部失败项/ })
    .click();
  await page
    .getByRole("button", {
      name: /Remove selected failed items \(2\)|移除所选失败项（2）/,
    })
    .click();

  await expect.poll(() => bulkDeleteBodies.length).toBe(1);
  expect(bulkDeleteBodies[0]).toEqual({ ids: [101, 102] });
  await expect(
    dialog.getByText(
      /Existing providers pending removal confirmation|待确认移除的已有提供商/,
    ),
  ).toHaveCount(0);
  await expect(
    dialog.getByRole("button", {
      name: /Remove selected failed items|移除所选失败项/,
    }),
  ).toHaveCount(0);

  const afterScreenshotPath = testInfo.outputPath(
    "claude-batch-test-after-bulk-remove.png",
  );
  await page.screenshot({ path: afterScreenshotPath, fullPage: true });
  await testInfo.attach("failed existing providers after bulk remove", {
    path: afterScreenshotPath,
    contentType: "image/png",
  });
});
