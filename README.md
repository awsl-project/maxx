<p align="center">
  <img src="web/public/logo.png" alt="maxx logo" width="128" height="128">
</p>

# maxx

English | [简体中文](README_CN.md)

Multi-provider AI proxy with a built-in admin UI, routing, and usage tracking.

## Features
- Proxy endpoints for Claude, OpenAI, Gemini, and Codex formats
- Admin API and Web UI
- Provider routing, retries, and quotas
- SQLite-backed storage

## Getting Started

### 1. Start the Service

Start the service using Docker Compose (recommended):

```bash
docker compose up -d
```

The service will run at `http://localhost:9880`.

**Full docker-compose.yml example:**

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

Service data is stored in the `/data` directory and persisted via volume.

### 2. Access Admin UI

Open your browser and visit [http://localhost:9880](http://localhost:9880) to access the Web admin interface.

### 3. Configure Claude Code

#### 3.1 Get API Key

Create a project in the maxx admin interface and generate an API key.

#### 3.2 Configure Environment Variables

**settings.json Configuration (Recommended, Permanent)**

Configuration location: `~/.claude/settings.json` or `.claude/settings.json`

```json
{
  "env": {
    "ANTHROPIC_AUTH_TOKEN": "your-api-key-here",
    "ANTHROPIC_BASE_URL": "http://localhost:9880"
  }
}
```

**Important Notes:**
- `ANTHROPIC_AUTH_TOKEN`: Can be any value (no real key required for local deployment)
- `ANTHROPIC_BASE_URL`: Use `http://localhost:9880` for local deployment

#### 3.3 Start Using

After configuration, Claude Code will access AI services through the maxx proxy. You can view usage and quotas in the admin interface.

## Local Development

### Server Mode (Browser)
Backend:
```bash
go run cmd/maxx/main.go
```

Frontend:
```bash
cd web
npm install
npm run dev
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
- Desktop mode (Windows): `%APPDATA%\maxx`
- Server mode (non-Docker): `~/.config/maxx/maxx.db`
- Docker data directory: `/data` (mounted via `docker-compose.yml`)

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
