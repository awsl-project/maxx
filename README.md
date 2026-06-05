<p align="center">
  <img src="web/public/logo.png" alt="maxx logo" width="128" height="128">
</p>

<p align="center">
  <a href="https://github.com/awsl-project/maxx/releases/latest"><img src="https://img.shields.io/github/v/release/awsl-project/maxx?display_name=tag&style=flat-square" alt="Latest Release"></a>
  <a href="https://github.com/awsl-project/maxx/pkgs/container/maxx"><img src="https://img.shields.io/badge/ghcr.io-awsl--project%2Fmaxx-blue?style=flat-square&logo=github" alt="GHCR Image"></a>
  <a href="https://github.com/awsl-project/maxx/blob/main/go.mod"><img src="https://img.shields.io/github/go-mod/go-version/awsl-project/maxx?style=flat-square" alt="Go Version"></a>
  <a href="https://github.com/awsl-project/maxx/actions/workflows/lint.yml"><img src="https://img.shields.io/github/actions/workflow/status/awsl-project/maxx/lint.yml?event=pull_request&label=Checks&style=flat-square" alt="Checks"></a>
  <a href="https://github.com/awsl-project/maxx/actions/workflows/e2e-test.yml"><img src="https://img.shields.io/github/actions/workflow/status/awsl-project/maxx/e2e-test.yml?branch=main&label=E2E&style=flat-square" alt="E2E Tests"></a>
  <a href="https://github.com/awsl-project/maxx/actions/workflows/e2e-playwright.yml"><img src="https://img.shields.io/github/actions/workflow/status/awsl-project/maxx/e2e-playwright.yml?event=pull_request&label=Playwright&style=flat-square" alt="Playwright Tests"></a>
</p>

<h1 align="center">maxx</h1>

<p align="center">
  English | <a href="README_CN.md">简体中文</a>
</p>

<p align="center">
  Multi-provider AI proxy with a built-in admin UI, routing, and usage tracking.
</p>

<p align="center">
  <a href="docs/database-migrations.md">Docs</a> · <a href="#docker-recommended-for-server">Docker</a> · <a href="#desktop-app-recommended-for-personal-use">Desktop</a> · <a href="#api-endpoints">API</a>
</p>

## Features

- **Protocol Compatibility**: Claude, OpenAI, Gemini, and Codex API formats
- **AI Tool Friendly**: Works with Claude Code, Codex CLI, and other coding agents
- **Provider Types**: Custom relay, Antigravity (Google), Kiro (AWS)
- **Routing**: Priority-based and weighted-random routing strategies
- **Databases**: SQLite (default), MySQL, PostgreSQL
- **Usage & Billing**: Request logs + nano-dollar pricing with multipliers
- **Pricing Catalog**: Versioned, tiered, and cache pricing support
- **Admin UI**: Multi-language Web UI with real-time WebSocket updates
- **Profiling**: Built-in pprof support
- **Backup**: Import/export configuration

## Quick Start

Maxx supports three deployment methods:

| Method | Description | Best For |
|--------|-------------|----------|
| **Docker** | Containerized deployment | Server/production |
| **Desktop App** | Native application with GUI | Personal use |
| **Local Build** | Build from source | Development |

### Docker (Recommended for Server)

```bash
docker compose up -d
```

The service will run at `http://localhost:9880`.

<details>
<summary>📄 Full docker-compose.yml example</summary>

```yaml
services:
  maxx:
    image: ghcr.io/awsl-project/maxx:latest
    container_name: maxx
    restart: unless-stopped
    ports:
      - "9880:9880"
    volumes:
      - maxx-data:/data
    environment:
      - MAXX_ADMIN_PASSWORD=your-password  # Optional: Enable admin authentication
    healthcheck:
      test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://localhost:9880/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 40s

volumes:
  maxx-data:
    driver: local
```

</details>

### Desktop App (Recommended for Personal Use)

Download from [GitHub Releases](https://github.com/awsl-project/maxx/releases):

| Platform | File | Notes |
|----------|------|-------|
| Windows | `maxx.exe` | Run directly |
| macOS (ARM) | `maxx-macOS-arm64.dmg` | Apple Silicon (M1/M2/M3/M4) |
| macOS (Intel) | `maxx-macOS-amd64.dmg` | Intel chips |
| Linux | `maxx` | Native binary |

<details>
<summary>🍺 macOS Homebrew Installation</summary>

```bash
# Install
brew install --cask awsl-project/awsl/maxx

# Upgrade
brew upgrade --cask awsl-project/awsl/maxx
```

> **Gatekeeper note:** maxx is not notarized. On first launch, macOS Gatekeeper may block it.
> To allow it, run:
> `xattr -d com.apple.quarantine /Applications/maxx.app`
>
> Or go to **System Settings > Privacy & Security** and click **Open Anyway**.
>
> **If macOS says the app is damaged:**
> 1. Remove quarantine attributes:
>    `sudo xattr -rd com.apple.quarantine /Applications/maxx.app`
> 2. Right-click `maxx.app` in Finder and choose **Open** once.
> 3. If it still fails, reinstall and retry:
>    `brew uninstall --cask awsl-project/awsl/maxx && brew install --cask awsl-project/awsl/maxx`

</details>

### Local Build

```bash
# Server mode
go run cmd/maxx/main.go

# With admin authentication
MAXX_ADMIN_PASSWORD=your-password go run cmd/maxx/main.go

# Desktop mode (Wails)
go install github.com/wailsapp/wails/v2/cmd/wails@latest
wails dev
```

**Frontend requirements:** Node.js 22.12.0+ within the 22.x line (see `.node-version` / `.nvmrc`) and pnpm 10.7.0 (locked via `web/package.json`).

### Headless admin CLI (`maxx-cli`)

`maxx-cli` is a separate binary that talks to a running maxx server's admin
HTTP API. It covers everything the web UI does — providers, API tokens,
routes (with weights), routing strategies (with sticky session affinity),
users, invite codes, and settings — and is designed for scripts, CI, and
AI agents.

Supported install paths:

```bash
# From source (latest tagged release):
go install github.com/awsl-project/maxx/cmd/maxx-cli@latest

# From source (local checkout):
task install:cli      # uses Taskfile; installs to $GOBIN

# Inside the official Docker image: maxx-cli is on PATH at /usr/local/bin/maxx-cli
docker exec <container> maxx-cli --help
```

Standalone binary release assets and Homebrew/Scoop manifests for
`maxx-cli` are not yet published; track [#585](https://github.com/awsl-project/maxx/pull/585)
for the follow-up.

First-time usage:

```bash
maxx-cli login --server http://localhost:9880 --username admin
maxx-cli help reference       # full command tree, auto-generated
maxx-cli -o json provider list
```

For the full agent-friendly briefing run `maxx-cli help reference`,
`maxx-cli help formatting`, and `maxx-cli help auth-config`.

## Configure AI Coding Tools

### Claude Code

Create a project in the maxx admin interface and generate an API key.

**settings.json (Recommended)**

Location: `~/.claude/settings.json` or `.claude/settings.json`

```json
{
  "env": {
    "ANTHROPIC_AUTH_TOKEN": "your-api-key-here",
    "ANTHROPIC_BASE_URL": "http://localhost:9880"
  }
}
```

<details>
<summary>🔧 Shell Function (Alternative)</summary>

Add to your shell profile (`~/.bashrc`, `~/.zshrc`, etc.):

```bash
claude_maxx() {
    export ANTHROPIC_BASE_URL="http://localhost:9880"
    export ANTHROPIC_AUTH_TOKEN="your-api-key-here"
    claude "$@"
}
```

Then use `claude_maxx` instead of `claude`.

</details>

<details>
<summary>🔐 Token Authentication</summary>

**When Token Authentication is Enabled:**
- Set `ANTHROPIC_AUTH_TOKEN` to a token created in the 'API Tokens' page (format: `maxx_xxx`)
- Claude Code will automatically add the `x-api-key` header to requests
- maxx will validate the token before processing requests

**When Token Authentication is Disabled:**
- You can set `ANTHROPIC_AUTH_TOKEN` to any value (e.g., `"dummy"`) or leave it empty
- maxx will not validate the token
- Suitable for internal networks or testing scenarios
- ⚠️ **Warning:** Disabling token authentication reduces security

</details>

### Codex CLI

**config.toml**

Add to `~/.codex/config.toml`:

```toml
# Optional: Set as default provider
model_provider = "maxx"

[model_providers.maxx]
name = "maxx"
base_url = "http://localhost:9880"
wire_api = "responses"
request_max_retries = 4
stream_max_retries = 10
stream_idle_timeout_ms = 300000
```

**auth.json**

Create or edit `~/.codex/auth.json`:

```json
{
  "OPENAI_API_KEY": "maxx_your_token_here"
}
```

**Usage:**

```bash
# Use --provider flag to specify
codex --provider maxx

# Or use directly if set as default provider
codex
```

<details>
<summary>🔐 Token Authentication</summary>

**When Token Authentication is Enabled:**
- Configure `OPENAI_API_KEY` in `auth.json` with a token created in the 'API Tokens' page (format: `maxx_xxx`)
- Codex CLI will automatically add the `Authorization: Bearer <token>` header to requests
- maxx will validate the token before processing requests

**When Token Authentication is Disabled:**
- You can set `OPENAI_API_KEY` in `auth.json` to any value (e.g., `"dummy"`)
- maxx will not validate the token
- Suitable for internal networks or testing scenarios
- ⚠️ **Warning:** Disabling token authentication reduces security

</details>

## API Endpoints

| Type | Endpoint |
|------|----------|
| Claude | `POST /v1/messages` |
| OpenAI | `POST /v1/chat/completions` |
| Codex | `POST /v1/responses` |
| Gemini | `POST /v1beta/models/{model}:generateContent` |
| Project Proxy | `/project/{project-slug}/v1/messages` (etc.) |
| Admin API | `/api/admin/*` |
| WebSocket | `ws://localhost:9880/ws` |
| Health Check | `GET /health` |
| Web UI | `http://localhost:9880/` |

## Configuration

### Environment Variables

| Variable | Description |
|----------|-------------|
| `MAXX_ADMIN_PASSWORD` | Enable admin authentication with JWT. Default username: `admin`, password: the value of this variable |
| `MAXX_DSN` | Database connection string |
| `MAXX_DATA_DIR` | Custom data directory path |
| `MAXX_DISABLE_UI` | Headless mode: when truthy (`1`/`true`/`yes`/`on`), do not serve the web UI — only the API and proxy endpoints are exposed. Equivalent to the `-no-ui` flag (the flag takes precedence when set). Project proxy routes (`/project/{slug}/...`) remain available. |
| `MAXX_CORS_ALLOW_ORIGINS` | Comma-separated list of allowed origins (or `*`) for cross-origin requests. Enables a separately-hosted frontend to point at this backend; unset disables CORS (same-origin only). |
| `MAXX_ROUTING_SEED_SALT` | Optional shared secret for the `weighted_random` routing strategy. If unset, each process generates its own random salt — anti-grinding still holds and Redis sticky bindings still converge after the first successful request, but the pre-sticky first-pick order for the same `(token, session)` can differ across instances. Set the **same value on every instance** when you need consistent first-pick behavior in multi-instance deployments. |

### Headless Mode (API-only, no Web UI)

Run maxx as a pure API gateway without serving the admin Web UI — useful for
server/production deployments where you configure everything through the Admin
API and want a smaller attack surface.

Enable it with **either** the `-no-ui` flag **or** the `MAXX_DISABLE_UI`
environment variable (the flag wins if both are set):

```bash
# Flag (local build)
maxx -no-ui

# Env var (Docker / compose)
docker run -e MAXX_DISABLE_UI=true -p 9880:9880 ghcr.io/awsl-project/maxx
```

In headless mode:

- `/` and all web UI routes return `404` (no static files are served).
- The API (`/api/admin/*`), proxy endpoints (`/v1/messages`, `/v1/chat/completions`, …), project proxy (`/project/{slug}/...`), `/health`, and `/ws` all keep working.
- Configure providers, routes, tokens, etc. via the Admin API. Set `MAXX_ADMIN_PASSWORD` to protect it.

### Separately-hosted Frontend (point the UI at a remote backend)

You can host the Web UI on one origin (e.g. a CDN, a dev server, or a headless
maxx's sibling) and have it talk to a backend on a **different** origin.

**1. Allow the frontend's origin on the backend** via CORS (otherwise the
browser blocks cross-origin requests):

```bash
# Single origin
MAXX_CORS_ALLOW_ORIGINS=https://ui.example.com maxx

# Multiple origins (comma-separated), or "*" to allow any
MAXX_CORS_ALLOW_ORIGINS=https://ui.example.com,http://localhost:3000 maxx
```

> ⚠️ **CORS is not a substitute for authentication.** `*` lets *any* website read
> and call your API from a browser, including the admin API. Only use `*` for
> trusted/local setups, and always set `MAXX_ADMIN_PASSWORD` so the admin API
> requires a token. Prefer listing explicit origins over `*`. maxx logs a warning
> at startup if `*` is combined with an unauthenticated admin API.

**2. Point the UI at the backend.** Open the Web UI and either:

- On the **login screen**, expand **Connection settings** and enter the backend URL (e.g. `https://api.example.com`); or
- After login, go to **Settings → Backend address**.

The value is stored in the browser (`localStorage`), so each user/browser can
target a different backend. Leave it empty to use the same origin that served
the page (the default). Build-time default: set `VITE_BACKEND_URL` when building
the frontend.

### System Settings

Configurable via Admin UI:

| Setting | Description | Default |
|---------|-------------|---------|
| `proxy_port` | Proxy server port | `9880` |
| `request_retention_hours` | Request log retention (hours) | `168` (7 days) |
| `request_detail_retention_seconds` | Request detail retention (seconds, unified — used when split is off) | `-1` (forever) |
| `request_detail_retention_split_enabled` | Configure success/failed retention separately | `false` |
| `request_detail_retention_seconds_success` | Success request detail retention (seconds, only when split=true) | falls back to unified |
| `request_detail_retention_seconds_failed` | Failed request detail retention (seconds, only when split=true) | falls back to unified |
| `timezone` | Timezone setting | `Asia/Shanghai` |
| `quota_refresh_interval` | Antigravity quota refresh (minutes) | `0` (disabled) |
| `auto_sort_antigravity` | Auto-sort Antigravity routes | `false` |
| `enable_pprof` | Enable pprof profiling | `false` |
| `pprof_port` | Pprof server port | `6060` |
| `pprof_password` | Pprof access password | (empty) |

### Database Configuration

Maxx supports SQLite (default), MySQL, and PostgreSQL.

<details>
<summary>🗄️ MySQL Configuration</summary>

```bash
export MAXX_DSN="mysql://user:password@tcp(host:port)/dbname?parseTime=true&charset=utf8mb4"

# Example
export MAXX_DSN="mysql://maxx:secret@tcp(127.0.0.1:3306)/maxx?parseTime=true&charset=utf8mb4"
```

**Docker Compose with MySQL:**

```yaml
services:
  maxx:
    image: ghcr.io/awsl-project/maxx:latest
    container_name: maxx
    restart: unless-stopped
    ports:
      - "9880:9880"
    environment:
      - MAXX_DSN=mysql://maxx:secret@tcp(mysql:3306)/maxx?parseTime=true&charset=utf8mb4
    depends_on:
      mysql:
        condition: service_healthy

  mysql:
    image: mysql:8.0
    container_name: maxx-mysql
    restart: unless-stopped
    environment:
      MYSQL_ROOT_PASSWORD: rootpassword
      MYSQL_DATABASE: maxx
      MYSQL_USER: maxx
      MYSQL_PASSWORD: secret
    volumes:
      - mysql-data:/var/lib/mysql
    healthcheck:
      test: ["CMD", "mysqladmin", "ping", "-h", "localhost"]
      interval: 10s
      timeout: 5s
      retries: 5

volumes:
  mysql-data:
    driver: local
```

</details>

<details>
<summary>🐘 PostgreSQL Configuration</summary>

```bash
export MAXX_DSN="postgres://user:password@host:port/dbname?sslmode=disable"

# Example
export MAXX_DSN="postgres://maxx:secret@127.0.0.1:5432/maxx?sslmode=disable"
```

</details>

### Data Storage Locations

| Deployment | Location |
|------------|----------|
| Docker | `/data` (mounted volume) |
| Desktop (Windows) | `%USERPROFILE%\AppData\Local\maxx\` |
| Desktop (macOS) | `~/Library/Application Support/maxx/` |
| Desktop (Linux) | `~/.local/share/maxx/` |
| Server (non-Docker) | `~/.config/maxx/maxx.db` |

## Local Development

<details>
<summary>🛠️ Development Setup</summary>

### Server Mode (Browser)

**Build frontend first:**
```bash
cd web
pnpm install
pnpm build
```

**Then run backend:**
```bash
go run cmd/maxx/main.go
```

**Or run frontend dev server (for development):**
```bash
cd web
pnpm dev
```

### Desktop Mode (Wails)

See `WAILS_README.md` for detailed documentation.

```bash
# Install Wails CLI
go install github.com/wailsapp/wails/v2/cmd/wails@latest

# Run desktop app
wails dev

# Build desktop app
wails build
```

</details>

## Release

<details>
<summary>📦 Release Process</summary>

### GitHub Actions (Recommended)

1. Go to the repository's [Actions](../../actions) page
2. Select the "Release" workflow
3. Click "Run workflow"
4. Enter the version number (e.g., `v1.0.0`)
5. Click "Run workflow" to execute

### Local Script

```bash
./release.sh <github_token> <version>

# Example
./release.sh ghp_xxxx v1.0.0
```

Both methods will automatically create a tag and generate release notes.

</details>

## Acknowledgements

Special thanks to [router-for-me/CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI) for its open-source contributions and inspiration for forwarding compatibility design.
