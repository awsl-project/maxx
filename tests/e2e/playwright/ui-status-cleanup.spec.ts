import { expect, test, type Page, type Route } from "playwright/test";

type ProviderPayload = {
  id: number;
  createdAt: string;
  updatedAt: string;
  type: string;
  name: string;
  supportedClientTypes: string[];
  supportModels?: string[];
  exposedModelsEnabled?: boolean;
  config: {
    quotaEnabled?: boolean;
    custom: {
      baseURL: string;
      apiKey: string;
    };
  };
};

function nowIso() {
  return new Date().toISOString();
}

function provider(id: number, name: string): ProviderPayload {
  return {
    id,
    createdAt: nowIso(),
    updatedAt: nowIso(),
    type: "custom",
    name,
    supportedClientTypes: ["openai"],
    supportModels: ["gpt-ui-cleanup"],
    exposedModelsEnabled: false,
    config: {
      quotaEnabled: false,
      custom: {
        baseURL: "https://mock-provider.example.test/v1",
        apiKey: "mock-key",
      },
    },
  };
}

async function json(route: Route, body: unknown, status = 200) {
  await route.fulfill({
    status,
    contentType: "application/json",
    body: JSON.stringify(body),
  });
}

async function installMocks(page: Page) {
  const providers = [provider(42, "OpenAI Quiet Row")];
  const routes = [
    {
      id: 7,
      createdAt: nowIso(),
      updatedAt: nowIso(),
      isEnabled: true,
      isNative: false,
      projectID: 0,
      clientType: "openai",
      providerID: 42,
      position: 1,
      weight: 1,
      retryConfigID: 0,
      modelMapping: {},
    },
  ];
  const mappings = [
    {
      id: 11,
      createdAt: nowIso(),
      updatedAt: nowIso(),
      scope: "global",
      clientType: "openai",
      providerType: "custom",
      providerID: 0,
      projectID: 0,
      routeID: 0,
      apiTokenID: 0,
      pattern: "gpt-source",
      target: "gpt-target",
      priority: 1000,
      isEnabled: true,
      isBuiltin: false,
    },
  ];
  const activeRequests = [
    {
      id: 100,
      createdAt: nowIso(),
      updatedAt: nowIso(),
      instanceID: "mock",
      requestID: "active-openai-1",
      sessionID: "session-1",
      clientType: "openai",
      requestModel: "gpt-ui-cleanup",
      mappedModel: "gpt-ui-cleanup",
      responseModel: "",
      reasoningEffort: "",
      startTime: nowIso(),
      endTime: "",
      duration: 0,
      ttft: 0,
      isStream: true,
      protocol: "sse",
      status: "IN_PROGRESS",
      statusCode: 0,
      requestInfo: null,
      responseInfo: null,
      error: "",
      proxyUpstreamAttemptCount: 1,
      finalProxyUpstreamAttemptID: 0,
      routeID: 7,
      providerID: 42,
      projectID: 0,
      inputTokenCount: 0,
      outputTokenCount: 0,
      cacheReadCount: 0,
      cacheWriteCount: 0,
      cache5mWriteCount: 0,
    },
  ];

  await page.addInitScript(() => {
    localStorage.setItem("maxx-admin-token", "mock-token");
    localStorage.setItem("maxx-ui-language", "zh");
  });

  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const { pathname } = url;
    const method = request.method();

    if (pathname === "/api/admin/auth/status") {
      return json(route, {
        authEnabled: true,
        user: { id: 1, username: "admin", tenantID: 1, role: "admin" },
      });
    }

    if (pathname === "/api/settings" || pathname === "/api/admin/settings") {
      return json(route, {
        api_token_auth_enabled: "true",
        force_project_binding: "false",
        ui_multitenant_enabled: "false",
      });
    }

    if (
      pathname === "/api/proxy-status" ||
      pathname === "/api/admin/proxy-status"
    ) {
      return json(route, {
        running: true,
        address: "127.0.0.1",
        port: 9880,
        version: "e2e",
      });
    }

    if (pathname === "/api/providers" || pathname === "/api/admin/providers")
      return json(route, providers);
    if (pathname === "/api/routes" || pathname === "/api/admin/routes")
      return json(route, routes);
    if (
      pathname === "/api/admin/model-mappings" ||
      pathname === "/api/model-mappings"
    )
      return json(route, mappings);
    if (
      pathname === "/api/admin/requests/active" ||
      pathname === "/api/requests/active"
    )
      return json(route, activeRequests);
    if (pathname === "/api/admin/provider-stats") return json(route, {});
    if (pathname === "/api/admin/streaming-requests/counts")
      return json(route, { "42:openai": 1 });
    if (pathname === "/api/admin/projects" || pathname === "/api/projects")
      return json(route, []);
    if (pathname === "/api/admin/routing-strategies") return json(route, []);
    if (pathname === "/api/admin/api-tokens" || pathname === "/api/api-tokens")
      return json(route, []);
    if (pathname === "/api/admin/response-models") return json(route, []);

    return json(route, { error: "Unmocked endpoint", pathname, method }, 404);
  });
}

test.use({
  viewport: { width: 1440, height: 1100 },
  locale: "zh-CN",
  video: "on",
});

test("provider route and model mapping tabs keep status controls quiet", async ({
  page,
}, testInfo) => {
  await installMocks(page);

  await page.goto("/providers", { waitUntil: "domcontentloaded" });
  await expect(page.getByText("OpenAI Quiet Row")).toBeVisible();
  await expect(
    page.getByRole("button", { name: /复制.*命令|Copy add command/i }),
  ).toHaveCount(0);
  await expect(page.getByText(/复制命令|Copy add command/i)).toHaveCount(0);
  await page.screenshot({
    path: testInfo.outputPath("01-provider-tab-no-copy-command.png"),
    fullPage: true,
  });

  await page.goto("/routes/openai", { waitUntil: "domcontentloaded" });
  await expect(page.getByText("OpenAI Quiet Row")).toBeVisible();
  await expect(
    page.getByLabel(/当前有 1 个活跃请求|1 active request/i),
  ).toBeVisible();
  await expect(page.getByText("请求中")).toHaveCount(0);
  await page.screenshot({
    path: testInfo.outputPath("02-route-tab-icon-only-active.png"),
    fullPage: true,
  });

  await page.goto("/model-mappings", { waitUntil: "domcontentloaded" });
  await expect(page.getByRole("button", { name: "gpt-source" })).toBeVisible();
  await expect(page.getByRole("switch")).toBeVisible();
  await expect(
    page.getByText(/^启用$|^禁用$|^Enabled$|^Disabled$/),
  ).toHaveCount(0);
  await page.screenshot({
    path: testInfo.outputPath("03-model-mapping-status-icon-only.png"),
    fullPage: true,
  });
});
