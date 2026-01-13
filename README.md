<p align="center">
  <img src="web/public/logo.png" alt="maxx logo" width="128" height="128">
</p>

# maxx

Multi-provider AI proxy with a built-in admin UI, routing, and usage tracking.

## Features
- Proxy endpoints for Claude, OpenAI, Gemini, and Codex formats
- Admin API and Web UI
- Provider routing, retries, and quotas
- SQLite-backed storage

## Docker Compose (recommended)
The service stores its data under `/data` in the container. The compose file
already mounts a named volume so the SQLite DB is persisted.

```
docker compose up -d
```

Full example:
```
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

## Local Development

### Server Mode (Browser)
Backend:
```
go run cmd/maxx/main.go
```

Frontend:
```
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

创建新版本发布：

```bash
./release.sh <github_token> <version>
```

示例：
```bash
./release.sh ghp_xxxx v1.0.0
```

脚本会自动创建 tag 并推送到 GitHub，然后创建 Release（自动生成 release notes）。
