<p align="center">
  <img src="web/public/logo.png" alt="maxx logo" width="128" height="128">
</p>

# maxx

English | [简体中文](README_CN.md)

Multi-provider AI proxy with a built-in admin UI, routing, and usage tracking.

## Features
- Proxy endpoints for Claude, OpenAI, Gemini, and Codex formats
- Compatible with Claude Code, Codex CLI, and other AI coding tools as a unified API proxy gateway
- Admin API and Web UI
- Provider routing, retries, and quotas
- SQLite-backed storage

## Getting Started

Maxx supports three deployment methods:

| Method | Description | Best For |
|--------|-------------|----------|
| **Docker** | Containerized deployment | Server/production use |
| **Desktop App** | Native application with GUI | Personal use, easy setup |
| **Local Build** | Build from source | Development |

### Method 1: Docker (Recommended for Server)

Start the service using Docker Compose:

```bash
docker compose up -d
```

The service will run at `http://localhost:9880`.

<details>
<summary>Full docker-compose.yml example</summary>

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

### Method 2: Desktop App (Recommended for Personal Use)

Download pre-built desktop applications from [GitHub Releases](https://github.com/awsl-project/maxx/releases).

| Platform | File | Notes |
|----------|------|-------|
| Windows | `maxx.exe` | Run directly |
| macOS (ARM) | `maxx-macOS-arm64.dmg` | Apple Silicon (M1/M2/M3) |
| macOS (Intel) | `maxx-macOS-amd64.dmg` | Intel chips |
| Linux | `maxx` | Native binary |

### Method 3: Local Build

```bash
# Run server mode
go run cmd/maxx/main.go

# Or run desktop mode with Wails
go install github.com/wailsapp/wails/v2/cmd/wails@latest
wails dev
```

## Configure AI Coding Tools

### Claude Code

Create a project in the maxx admin interface and generate an API key, then configure Claude Code using one of the following methods:

**settings.json (Recommended)**

Configuration location: `~/.claude/settings.json` or `.claude/settings.json`

```json
{
  "env": {
    "ANTHROPIC_AUTH_TOKEN": "your-api-key-here",
    "ANTHROPIC_BASE_URL": "http://localhost:9880"
  }
}
```

**Shell Function (Alternative)**

Add to your shell profile (`~/.bashrc`, `~/.zshrc`, etc.):

```bash
claude_maxx() {
    export ANTHROPIC_BASE_URL="http://localhost:9880"
    export ANTHROPIC_AUTH_TOKEN="your-api-key-here"
    claude "$@"
}
```

Then use `claude_maxx` instead of `claude` to run Claude Code through maxx.

> **Note:** `ANTHROPIC_AUTH_TOKEN` can be any value for local deployment.

### Codex CLI

Add the following to your `~/.codex/config.toml`:

```toml
[model_providers.maxx]
name = "maxx"
base_url = "http://localhost:9880"
wire_api = "responses"
request_max_retries = 4
stream_max_retries = 10
stream_idle_timeout_ms = 300000
```

Then use `--provider maxx` when running Codex CLI.

## Local Development

### Server Mode (Browser)
Backend:
```bash
go run cmd/maxx/main.go
```

Frontend:
```bash
cd web
pnpm install
pnpm dev
```

### Desktop Mode (Wails)
See `WAILS_README.md` for detailed desktop app documentation.

Quick start:
```bash
# Install Wails CLI
go install github.com/wailsapp/wails/v2/cmd/wails@latest

# Run desktop app
wails dev

# Build desktop app
wails build
# or
build-desktop.bat
```

## Endpoints
- Admin API: http://localhost:9880/admin/
- Web UI: http://localhost:9880/
- WebSocket: ws://localhost:9880/ws
- Claude: http://localhost:9880/v1/messages
- OpenAI: http://localhost:9880/v1/chat/completions
- Codex: http://localhost:9880/v1/responses
- Gemini: http://localhost:9880/v1beta/models/{model}:generateContent
- Project proxy: http://localhost:9880/{project-slug}/v1/messages (etc.)

## Data

| Deployment | Data Location |
|------------|---------------|
| Docker | `/data` (mounted via volume) |
| Desktop (Windows) | `%USERPROFILE%\AppData\Local\maxx\` |
| Desktop (macOS) | `~/Library/Application Support/maxx/` |
| Desktop (Linux) | `~/.local/share/maxx/` |
| Server (non-Docker) | `~/.config/maxx/maxx.db` |

## Release

There are two ways to create a new release:

### GitHub Actions (Recommended)

1. Go to the repository's [Actions](../../actions) page
2. Select the "Release" workflow
3. Click "Run workflow"
4. Enter the version number (e.g., `v1.0.0`)
5. Click "Run workflow" to execute

### Local Script

```bash
./release.sh <github_token> <version>
```

Example:
```bash
./release.sh ghp_xxxx v1.0.0
```

Both methods will automatically create a tag and generate release notes.
