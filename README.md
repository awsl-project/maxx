<p align="center">
  <img src="web/public/logo.png" alt="maxx logo" width="128" height="128">
</p>

# maxx

English | [简体中文](README_CN.md)

Multi-provider AI proxy with a built-in admin UI, routing, and usage tracking.

## Features

- **Multi-Protocol Proxy**: Claude, OpenAI, Gemini, and Codex API formats
- **AI Coding Tool Support**: Compatible with Claude Code, Codex CLI, and other AI coding tools
- **Provider Management**: Custom relay, Antigravity (Google), Kiro (AWS) provider types
- **Smart Routing**: Priority-based and weighted random routing strategies
- **Multi-Database**: SQLite (default), MySQL, and PostgreSQL support
- **Usage Tracking**: Nano-dollar precision billing with request multiplier tracking
- **Model Pricing**: Versioned pricing with tiered and cache pricing support
- **Admin Interface**: Web UI with multi-language support and real-time WebSocket updates
- **Performance Profiling**: Built-in pprof support for debugging
- **Backup & Restore**: Configuration import/export functionality

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
brew install --no-quarantine awsl-project/awsl/maxx

# Upgrade
brew upgrade --no-quarantine awsl-project/awsl/maxx
```

> **Note:** If you see "App is damaged" error, run: `sudo xattr -d com.apple.quarantine /Applications/maxx.app`

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

### Codex CLI

Add to `~/.codex/config.toml`:

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

## API Endpoints

| Type | Endpoint |
|------|----------|
| Claude | `POST /v1/messages` |
| OpenAI | `POST /v1/chat/completions` |
| Codex | `POST /v1/responses` |
| Gemini | `POST /v1beta/models/{model}:generateContent` |
| Project Proxy | `/{project-slug}/v1/messages` (etc.) |
| Admin API | `/api/admin/*` |
| WebSocket | `ws://localhost:9880/ws` |
| Health Check | `GET /health` |
| Web UI | `http://localhost:9880/` |

## Configuration

### Environment Variables

| Variable | Description |
|----------|-------------|
| `MAXX_ADMIN_PASSWORD` | Enable admin authentication with JWT |
| `MAXX_DSN` | Database connection string |
| `MAXX_DATA_DIR` | Custom data directory path |

### System Settings

Configurable via Admin UI:

| Setting | Description | Default |
|---------|-------------|---------|
| `proxy_port` | Proxy server port | `9880` |
| `request_retention_hours` | Request log retention (hours) | `168` (7 days) |
| `request_detail_retention_seconds` | Request detail retention (seconds) | `-1` (forever) |
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
