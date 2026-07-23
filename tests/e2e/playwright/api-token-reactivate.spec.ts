import http from "node:http";

import { expect, test } from "playwright/test";

import {
  BASE,
  adminAPI,
  closeServer,
  loginToAdminAPI,
  loginToAdminUI,
} from "./helpers";

async function startMockOpenAIServer(): Promise<{
  server: http.Server;
  port: number;
  requests: string[];
}> {
  const requests: string[] = [];

  return new Promise((resolve) => {
    const server = http.createServer((req, res) => {
      if (req.method === "POST" && req.url?.includes("/v1/chat/completions")) {
        let body = "";
        req.on("data", (chunk) => {
          body += chunk;
        });
        req.on("end", () => {
          requests.push(body);
          res.writeHead(200, { "Content-Type": "application/json" });
          res.end(
            JSON.stringify({
              id: `chatcmpl_token_reactivate_${Date.now()}`,
              object: "chat.completion",
              created: Math.floor(Date.now() / 1000),
              model: "gpt-4o-mini",
              choices: [
                {
                  index: 0,
                  message: {
                    role: "assistant",
                    content: "mock token reactivate ok",
                  },
                  finish_reason: "stop",
                },
              ],
              usage: {
                prompt_tokens: 7,
                completion_tokens: 5,
                total_tokens: 12,
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
      resolve({ server, port: address.port, requests });
    });
  });
}

async function sendOpenAIProxyRequest(
  apiToken: string,
  marker: string,
): Promise<Response> {
  return fetch(`${BASE}/v1/chat/completions`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${apiToken}`,
    },
    body: JSON.stringify({
      model: "gpt-4o-mini",
      messages: [{ role: "user", content: marker }],
      max_tokens: 32,
    }),
  });
}

test("expired API token can be reactivated from the token tab and used for proxy requests", async ({
  page,
}, testInfo) => {
  const adminToken = await loginToAdminAPI();
  const mock = await startMockOpenAIServer();
  const tokenName = `expired-reactivate-${Date.now()}`;
  let providerId: number | null = null;
  let routeId: number | null = null;
  let tokenId: number | null = null;

  try {
    await adminAPI(
      "PUT",
      "/settings/api_token_auth_enabled",
      { value: "true" },
      adminToken,
    );

    const provider = await adminAPI(
      "POST",
      "/providers",
      {
        name: `Token Reactivate Mock OpenAI ${Date.now()}`,
        type: "custom",
        config: {
          custom: {
            baseURL: `http://127.0.0.1:${mock.port}`,
            apiKey: "mock-provider-key",
          },
        },
        supportedClientTypes: ["openai"],
        supportModels: ["*"],
      },
      adminToken,
    );
    providerId = provider.id;

    const route = await adminAPI(
      "POST",
      "/routes",
      {
        isEnabled: true,
        isNative: false,
        clientType: "openai",
        providerID: provider.id,
        projectID: 0,
        position: 1,
      },
      adminToken,
    );
    routeId = route.id;

    const expiredToken = await adminAPI(
      "POST",
      "/api-tokens",
      {
        name: tokenName,
        description: "E2E expired token reactivation",
        projectID: 0,
        expiresAt: "2020-01-01T00:00:00.000Z",
      },
      adminToken,
    );
    tokenId = expiredToken.apiToken.id;
    const rawToken = expiredToken.token as string;

    const rejected = await sendOpenAIProxyRequest(
      rawToken,
      "before-reactivate",
    );
    expect(rejected.status).toBe(401);
    expect(mock.requests).toHaveLength(0);

    await loginToAdminUI(page);
    await page.goto(`${BASE}/api-tokens`);
    await expect(page.getByText(tokenName)).toBeVisible({ timeout: 15000 });
    await expect(page.getByText(/Expired|已过期/)).toBeVisible();

    const expiredScreenshot =
      "/tmp/maxx-api-token-expired-before-reactivate.png";
    await page.screenshot({ path: expiredScreenshot, fullPage: true });
    await testInfo.attach("api-token-expired-before-reactivate", {
      path: expiredScreenshot,
      contentType: "image/png",
    });

    await page
      .getByRole("button", { name: /Reset validity|Reactivate|恢复有效/ })
      .first()
      .click();

    await expect
      .poll(
        async () => {
          const token = await adminAPI(
            "GET",
            `/api-tokens/${tokenId}`,
            undefined,
            adminToken,
          );
          return {
            isEnabled: token.isEnabled,
            expiresAt: token.expiresAt ?? null,
            lastUsedAt: token.lastUsedAt ?? null,
          };
        },
        { timeout: 15000 },
      )
      .toEqual({ isEnabled: true, expiresAt: null, lastUsedAt: null });

    await expect(page.getByText(tokenName)).toBeVisible();
    await expect(page.getByText(/Active|启用|正常/)).toBeVisible();

    const activeScreenshot = "/tmp/maxx-api-token-active-after-reactivate.png";
    await page.screenshot({ path: activeScreenshot, fullPage: true });
    await testInfo.attach("api-token-active-after-reactivate", {
      path: activeScreenshot,
      contentType: "image/png",
    });

    const marker = `token reactivate marker ${Date.now()}`;
    const accepted = await sendOpenAIProxyRequest(rawToken, marker);
    expect(accepted.status).toBe(200);
    const acceptedBody = await accepted.json();
    expect(acceptedBody.choices?.[0]?.message?.content).toBe(
      "mock token reactivate ok",
    );
    expect(mock.requests).toHaveLength(1);
    expect(mock.requests[0]).toContain(marker);

    const proxyScreenshot = "/tmp/maxx-api-token-reactivated-proxy-ok.png";
    await page.evaluate(
      ({ markerText, mockCount }) => {
        const panel = document.createElement("section");
        panel.setAttribute("data-testid", "token-reactivate-proxy-evidence");
        panel.style.cssText =
          "position:fixed;left:24px;right:24px;bottom:24px;z-index:9999;padding:16px;border:2px solid #16a34a;border-radius:12px;background:white;color:#111;font:13px ui-monospace,monospace;box-shadow:0 12px 30px rgba(0,0,0,.18)";
        panel.textContent = `Reactivated token proxy request succeeded; mockCount=${mockCount}; marker=${markerText}`;
        document.body.appendChild(panel);
      },
      { markerText: marker, mockCount: mock.requests.length },
    );
    await page.screenshot({ path: proxyScreenshot, fullPage: true });
    await testInfo.attach("api-token-reactivated-proxy-ok", {
      path: proxyScreenshot,
      contentType: "image/png",
    });
  } finally {
    if (routeId) {
      await adminAPI(
        "DELETE",
        `/routes/${routeId}`,
        undefined,
        adminToken,
      ).catch(() => undefined);
    }
    if (providerId) {
      await adminAPI(
        "DELETE",
        `/providers/${providerId}`,
        undefined,
        adminToken,
      ).catch(() => undefined);
    }
    if (tokenId) {
      await adminAPI(
        "DELETE",
        `/api-tokens/${tokenId}`,
        undefined,
        adminToken,
      ).catch(() => undefined);
    }
    await closeServer(mock.server).catch(() => undefined);
  }
});
