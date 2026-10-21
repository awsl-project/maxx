import http from "node:http";

import { expect, test, type Page } from "playwright/test";

import {
  BASE,
  adminAPI,
  closeServer,
  loginToAdminAPI,
  loginToAdminUI,
} from "./helpers";

const REQUEST_MODEL = "claude-client-display-model";
const MAPPED_MODEL = "mock-upstream-mapped-model";
const UPSTREAM_RESPONSE_MODEL = "mock-upstream-response-model";
const REQUESTS_SCREENSHOT = "/tmp/maxx-request-model-chain-requests.png";
const MOCK_SCREENSHOT = "/tmp/maxx-request-model-chain-mock.png";

test.describe.configure({ mode: "serial" });

type MockRequestLog = {
  method: string;
  url: string;
  model: string;
  body: unknown;
};

function startMockClaudeServer(): Promise<{
  server: http.Server;
  port: number;
  logs: MockRequestLog[];
}> {
  const logs: MockRequestLog[] = [];

  return new Promise((resolve) => {
    const server = http.createServer((req, res) => {
      if (req.method === "GET" && req.url === "/__mock-log") {
        res.writeHead(200, { "Content-Type": "text/html; charset=utf-8" });
        res.end(`<!doctype html>
          <html>
            <head><title>Mock upstream interaction log</title></head>
            <body style="font-family: sans-serif; padding: 24px;">
              <h1>Mock upstream interaction log</h1>
              <p>Received model: <strong>${logs[0]?.model ?? ""}</strong></p>
              <p>Returned response model: <strong>${UPSTREAM_RESPONSE_MODEL}</strong></p>
              <pre>${JSON.stringify(logs, null, 2)}</pre>
            </body>
          </html>`);
        return;
      }

      if (req.method === "POST" && req.url?.includes("/v1/messages")) {
        let body = "";
        req.on("data", (chunk) => {
          body += chunk;
        });
        req.on("end", () => {
          let parsed: any = {};
          try {
            parsed = JSON.parse(body);
          } catch {
            // Keep the log useful even if a regression sends malformed JSON.
          }

          logs.push({
            method: req.method || "POST",
            url: req.url || "",
            model: parsed.model || "",
            body: parsed,
          });

          res.writeHead(200, { "Content-Type": "application/json" });
          res.end(
            JSON.stringify({
              id: `msg_model_chain_${Date.now()}`,
              type: "message",
              role: "assistant",
              model: UPSTREAM_RESPONSE_MODEL,
              content: [
                { type: "text", text: "mock response for model chain display" },
              ],
              stop_reason: "end_turn",
              stop_sequence: null,
              usage: {
                input_tokens: 11,
                output_tokens: 7,
                cache_creation_input_tokens: 0,
                cache_read_input_tokens: 0,
              },
            }),
          );
        });
        return;
      }

      res.writeHead(404, { "Content-Type": "application/json" });
      res.end(JSON.stringify({ error: "not found" }));
    });

    server.listen(0, "127.0.0.1", () => {
      const address = server.address();
      if (!address || typeof address === "string") {
        throw new Error("Failed to determine mock server port");
      }
      resolve({ server, port: address.port, logs });
    });
  });
}

async function sendClaudeRequest() {
  const response = await fetch(`${BASE}/v1/messages`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "anthropic-version": "2023-06-01",
    },
    body: JSON.stringify({
      model: REQUEST_MODEL,
      max_tokens: 100,
      messages: [
        { role: "user", content: "verify request list model mapping display" },
      ],
    }),
  });

  const text = await response.text();
  if (!response.ok) {
    throw new Error(`Proxy request failed (${response.status}): ${text}`);
  }
  return JSON.parse(text);
}

async function openRequestsPage(page: Page) {
  const passwordInput = page.locator('input[type="password"]');
  const requestsHeading = page.getByRole("heading", { name: "Requests" });

  await page.goto(`${BASE}/requests`, { waitUntil: "domcontentloaded" });
  await Promise.race([
    requestsHeading
      .waitFor({ state: "visible", timeout: 10_000 })
      .catch(() => undefined),
    passwordInput
      .waitFor({ state: "visible", timeout: 10_000 })
      .catch(() => undefined),
  ]);

  if (await passwordInput.isVisible().catch(() => false)) {
    await loginToAdminUI(page);
    await page.goto(`${BASE}/requests`, { waitUntil: "domcontentloaded" });
  }

  await expect(requestsHeading).toBeVisible({ timeout: 30_000 });
}

test("requests display mapped models and detail panels resize across desktop and mobile", async ({
  page,
}, testInfo) => {
  test.setTimeout(180_000);

  const mock = await startMockClaudeServer();
  let jwt: string | undefined;
  let providerId: number | null = null;
  let routeId: number | null = null;
  let modelMappingId: number | null = null;
  let previousApiTokenAuthEnabled: string | undefined;

  try {
    jwt = await loginToAdminAPI();
    const settings = await adminAPI("GET", "/settings", undefined, jwt);
    previousApiTokenAuthEnabled = settings.api_token_auth_enabled;
    await adminAPI(
      "PUT",
      "/settings/api_token_auth_enabled",
      { value: "false" },
      jwt,
    );

    const suffix = Date.now();
    const provider = await adminAPI(
      "POST",
      "/providers",
      {
        name: `Model Chain Mock ${suffix}`,
        type: "custom",
        config: {
          custom: {
            baseURL: `http://127.0.0.1:${mock.port}`,
            apiKey: "mock-key",
            responseModelMapping: {
              [UPSTREAM_RESPONSE_MODEL]: REQUEST_MODEL,
            },
          },
        },
        supportedClientTypes: ["claude"],
        supportModels: ["*"],
      },
      jwt,
    );
    providerId = provider.id;

    const route = await adminAPI(
      "POST",
      "/routes",
      {
        isEnabled: true,
        isNative: false,
        clientType: "claude",
        providerID: provider.id,
        projectID: 0,
        position: 1,
      },
      jwt,
    );
    routeId = route.id;

    const modelMapping = await adminAPI(
      "POST",
      "/model-mappings",
      {
        scope: "provider",
        clientType: "claude",
        providerID: provider.id,
        pattern: REQUEST_MODEL,
        target: MAPPED_MODEL,
        priority: 1,
      },
      jwt,
    );
    modelMappingId = modelMapping.id;

    const proxyResponse = await sendClaudeRequest();
    expect(proxyResponse.model).toBe(REQUEST_MODEL);
    expect(mock.logs[0]?.model).toBe(MAPPED_MODEL);

    await expect
      .poll(
        async () => {
          const requests = await adminAPI(
            "GET",
            `/requests?limit=10&providerId=${providerId}`,
            undefined,
            jwt,
          );
          const item = requests.items?.find(
            (candidate: any) => candidate.requestModel === REQUEST_MODEL,
          );
          if (!item) {
            return null;
          }
          return {
            requestModel: item.requestModel,
            mappedModel: item.mappedModel,
            responseModel: item.responseModel,
          };
        },
        { timeout: 20_000 },
      )
      .toEqual({
        requestModel: REQUEST_MODEL,
        mappedModel: MAPPED_MODEL,
        responseModel: UPSTREAM_RESPONSE_MODEL,
      });

    await openRequestsPage(page);
    const firstRow = page.locator('tbody tr[data-request-row="true"]').first();
    await expect(firstRow).toContainText(REQUEST_MODEL, { timeout: 30_000 });
    await expect(firstRow).toContainText(MAPPED_MODEL);
    await expect(firstRow).not.toContainText("response:");
    await expect(firstRow).not.toContainText(UPSTREAM_RESPONSE_MODEL);
    await page.screenshot({ path: REQUESTS_SCREENSHOT, fullPage: true });
    await testInfo.attach("requests-list-model-chain.png", {
      path: REQUESTS_SCREENSHOT,
      contentType: "image/png",
    });

    await page.setViewportSize({ width: 1440, height: 1000 });
    await firstRow.click();
    await expect(page).toHaveURL(/\/requests\/\d+$/);
    const group = page.locator('[data-slot="resizable-panel-group"]');
    const sidebar = group.locator('[data-slot="resizable-panel"]').first();
    const separator = group.getByRole("separator");
    const sidebarPercent = () => sidebar.evaluate((panel) =>
      panel.getBoundingClientRect().width / panel.parentElement!.getBoundingClientRect().width * 100,
    );
    await expect(separator).toBeVisible();
    await expect.poll(sidebarPercent).toBeCloseTo(20, 0);
    await page.screenshot({ path: testInfo.outputPath("request-detail-default.png"), animations: "disabled" });

    // Numeric sizes changed from percentages to pixels in react-resizable-panels v4.
    // Exercise actual geometry and input rather than only checking the props.
    const handle = await separator.boundingBox();
    const bounds = await group.boundingBox();
    expect(handle).not.toBeNull();
    expect(bounds).not.toBeNull();
    await page.mouse.move(handle!.x + handle!.width / 2, handle!.y + handle!.height / 2);
    await page.mouse.down();
    await page.mouse.move(bounds!.x + bounds!.width * 0.35, handle!.y + handle!.height / 2, { steps: 10 });
    await page.mouse.up();
    await expect.poll(sidebarPercent).toBeGreaterThan(30);
    await expect.poll(sidebarPercent).toBeLessThan(40);
    await page.screenshot({ path: testInfo.outputPath("request-detail-resized.png"), animations: "disabled" });

    await separator.focus();
    await page.keyboard.press("Home");
    await expect.poll(sidebarPercent).toBeCloseTo(20, 0);
    await page.keyboard.press("End");
    await expect.poll(sidebarPercent).toBeCloseTo(50, 0);

    await page.setViewportSize({ width: 390, height: 844 });
    await expect(group).toHaveCount(0);
    await page.getByRole("tab", { name: "Detail", exact: true }).click();
    await expect(page.getByRole("tab", { name: "Request", exact: true })).toBeVisible();
    await page.screenshot({ path: testInfo.outputPath("request-detail-mobile.png"), animations: "disabled" });

    const mockPage = await page.context().newPage();
    await mockPage.goto(`http://127.0.0.1:${mock.port}/__mock-log`);
    await expect(
      mockPage.getByText(`Received model: ${MAPPED_MODEL}`),
    ).toBeVisible();
    await mockPage.screenshot({ path: MOCK_SCREENSHOT, fullPage: true });
    await testInfo.attach("mock-upstream-interaction.png", {
      path: MOCK_SCREENSHOT,
      contentType: "image/png",
    });
  } finally {
    if (previousApiTokenAuthEnabled !== undefined) {
      try {
        await adminAPI(
          "PUT",
          "/settings/api_token_auth_enabled",
          { value: previousApiTokenAuthEnabled },
          jwt,
        );
      } catch {}
    }
    if (modelMappingId) {
      try {
        await adminAPI(
          "DELETE",
          `/model-mappings/${modelMappingId}`,
          undefined,
          jwt,
        );
      } catch {}
    }
    if (routeId) {
      try {
        await adminAPI("DELETE", `/routes/${routeId}`, undefined, jwt);
      } catch {}
    }
    if (providerId) {
      try {
        await adminAPI("DELETE", `/providers/${providerId}`, undefined, jwt);
      } catch {}
    }
    await closeServer(mock.server);
  }
});
