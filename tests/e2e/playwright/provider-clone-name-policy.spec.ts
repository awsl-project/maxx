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
  exposedModels?: string[];
  excludeFromExport?: boolean;
  blackBox?: boolean;
  config: {
    quotaEnabled?: boolean;
    disableErrorCooldown?: boolean;
    smartMappingRetryEnabled?: boolean;
    smartMappingRetryLimit?: number;
    custom: {
      baseURL: string;
      apiKey: string;
      responseModelMapping?: Record<string, string>;
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
    supportModels: ["gpt-clone-e2e"],
    exposedModelsEnabled: false,
    config: {
      quotaEnabled: false,
      disableErrorCooldown: false,
      smartMappingRetryEnabled: false,
      smartMappingRetryLimit: 1,
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

async function installMocks(page: Page, createdPayloads: unknown[]) {
  const settings: Record<string, string> = {
    api_token_auth_enabled: "true",
    force_project_binding: "false",
    ui_multitenant_enabled: "false",
  };
  const providers = [provider(42, "OpenAI 009"), provider(99, "OpenAI 010")];

  await page.addInitScript(() => {
    localStorage.setItem("maxx-admin-token", "mock-token");
    localStorage.setItem("maxx-ui-language", "en");
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
      if (method === "GET") return json(route, settings);
    }

    const settingMatch = pathname.match(/^\/api\/admin\/settings\/(.+)$/u);
    if (settingMatch && (method === "PUT" || method === "POST")) {
      const body = JSON.parse(request.postData() || "{}") as { value?: string };
      settings[decodeURIComponent(settingMatch[1])] = body.value || "";
      return json(route, {
        key: decodeURIComponent(settingMatch[1]),
        value: body.value || "",
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

    if (pathname === "/api/providers" || pathname === "/api/admin/providers") {
      if (method === "GET") return json(route, providers);
      if (method === "POST") {
        const payload = JSON.parse(request.postData() || "{}");
        createdPayloads.push(payload);
        const created = {
          ...provider(100 + createdPayloads.length, payload.name),
          ...payload,
        };
        providers.push(created);
        return json(route, created, 201);
      }
    }

    if (
      pathname === "/api/providers/42" ||
      pathname === "/api/admin/providers/42"
    ) {
      if (method === "GET") return json(route, providers[0]);
    }

    if (
      pathname === "/api/admin/provider-stats" ||
      pathname === "/api/admin/streaming-requests/counts" ||
      pathname === "/api/admin/routes" ||
      pathname === "/api/admin/model-mappings" ||
      pathname === "/api/admin/response-models" ||
      pathname === "/api/admin/projects" ||
      pathname === "/api/projects" ||
      pathname === "/api/admin/api-tokens" ||
      pathname === "/api/api-tokens"
    ) {
      return json(route, pathname.includes("counts") ? {} : []);
    }

    return json(route, { error: "Unmocked endpoint", pathname, method }, 404);
  });
}

test.use({
  viewport: { width: 1440, height: 1100 },
  locale: "en-US",
  video: "on",
});

test("provider clone naming follows configured numeric policy without regressing old clone flow", async ({
  page,
}, testInfo) => {
  const createdPayloads: unknown[] = [];
  await installMocks(page, createdPayloads);

  await page.goto("/settings", { waitUntil: "domcontentloaded" });
  await expect(page.getByText("Provider clone naming")).toBeVisible();
  await page.getByLabel("Clone naming strategy").click();
  await page.getByRole("option", { name: "Increment trailing number" }).click();
  await expect(
    page.getByText("Number increment advances a trailing number"),
  ).toBeVisible();
  await page.screenshot({
    path: testInfo.outputPath("01-clone-naming-setting.png"),
    fullPage: true,
  });

  await page.goto("/providers/42/edit", { waitUntil: "domcontentloaded" });
  await expect(page.locator('input[value="OpenAI 009"]')).toBeVisible();
  await page.getByRole("button", { name: /^Clone$/ }).click();

  await expect.poll(() => createdPayloads.length, { timeout: 10000 }).toBe(1);
  expect(createdPayloads[0]).toMatchObject({
    name: "OpenAI 011",
    type: "custom",
    supportedClientTypes: ["openai"],
    supportModels: ["gpt-clone-e2e"],
  });

  await expect(page).toHaveURL(/\/providers\/101\/edit/);
  await expect(page.locator('input[value="OpenAI 011"]')).toBeVisible();
  await page.screenshot({
    path: testInfo.outputPath("02-provider-cloned-with-numeric-policy.png"),
    fullPage: true,
  });
});
