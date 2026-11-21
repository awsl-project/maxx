# Get a Claude/Codex request through maxx in 5 minutes

This guide is for first-time users who want to prove maxx is useful before learning every tab.

By the end you will have:

1. a running maxx server,
2. one upstream provider,
3. one route,
4. one API token,
5. Claude Code or Codex CLI sending a request through maxx.

## 1. Start maxx

For a local evaluation, run the container with a persistent volume:

```bash
docker run --rm -p 9880:9880 -v maxx-data:/data ghcr.io/awsl-project/maxx:latest
```

Open `http://localhost:9880`.

For a shared server, set an admin password before exposing the service:

```bash
docker run -d \
  --name maxx \
  --restart unless-stopped \
  -p 9880:9880 \
  -v maxx-data:/data \
  -e MAXX_ADMIN_PASSWORD='change-me' \
  ghcr.io/awsl-project/maxx:latest
```

## 2. Add a provider

In the admin UI:

1. Open **Providers**.
2. Add your upstream provider.
3. Fill in the provider base URL, API key, and supported client type.
4. Save it.
5. Use the provider test action if available.

If your upstream is OpenAI-compatible, choose the OpenAI-compatible/custom relay path and keep the provider secret inside maxx.

## 3. Create a route

Open **Routes** and create a route for the client protocol you will use first:

- **Claude Code**: create a Claude route.
- **Codex CLI**: create an OpenAI Responses route.
- **Generic OpenAI client**: create an OpenAI route.

Start with one provider and one route. Add fallback and weighting only after the first request works.

## 4. Create an API token

Open **API Tokens** and create a token for your local tool or user.

Copy the token once. It should look like `maxx_...`.

## 5. Configure your tool

### Claude Code

Put this in `~/.claude/settings.json` or `.claude/settings.json`:

```json
{
  "env": {
    "ANTHROPIC_AUTH_TOKEN": "maxx_your_token_here",
    "ANTHROPIC_BASE_URL": "http://localhost:9880"
  }
}
```

Then run Claude Code and send a small request.

### Codex CLI

Put this in `~/.codex/config.toml`:

```toml
model_provider = "maxx"

[model_providers.maxx]
name = "maxx"
base_url = "http://localhost:9880"
wire_api = "responses"
requires_openai_auth = true
supports_websockets = true
request_max_retries = 4
stream_max_retries = 10
stream_idle_timeout_ms = 300000
```

Use the maxx API token as the OpenAI auth token for Codex, then send a small request.

## 6. Verify the request

Back in maxx, open **Requests**.

You should see the tool request, selected route/provider, status, latency, and usage details. If the request does not appear, the tool is not pointing at maxx yet.

## Troubleshooting

- **401 or unauthorized**: the local tool token is missing or not the `maxx_...` API token created in maxx.
- **404 or route not found**: create a route for the protocol your client actually uses.
- **Provider error**: test the provider in maxx first, then inspect the request detail.
- **Codex hangs on long tasks**: keep the WebSocket settings from the example and check the request detail page.
- **No request appears**: the client is still using its original provider URL instead of `http://localhost:9880`.

## What to configure next

After the first request works, add:

- fallback providers,
- weighted routing,
- model price entries,
- per-user tokens and quotas,
- request retention settings,
- backups for provider and route configuration.
