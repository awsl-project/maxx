<p align="center">
  <img src="web/public/logo.png" alt="maxx logo" width="128" height="128">
</p>

# maxx

[English](README.md) | 简体中文

多提供商 AI 代理服务，内置管理界面、路由和使用追踪功能。

## 功能特性
- 支持 Claude、OpenAI、Gemini 和 Codex 格式的代理端点
- 管理 API 和 Web UI
- 提供商路由、重试和配额管理
- 基于 SQLite 的数据存储

## 如何使用

### 1. 启动服务

使用 Docker Compose 启动服务（推荐）：

```bash
docker compose up -d
```

服务将在 `http://localhost:9880` 上运行。

**完整的 docker-compose.yml 示例：**

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

服务数据存储在 `/data` 目录下，通过 volume 持久化。

### 2. 访问管理界面

打开浏览器访问 [http://localhost:9880](http://localhost:9880) 进入 Web 管理界面。

### 3. 配置 Claude Code

#### 3.1 获取 API 密钥

在 maxx 管理界面中创建项目并生成 API 密钥。

#### 3.2 配置环境变量

**settings.json 配置（推荐，永久生效）**

配置位置：`~/.claude/settings.json` 或 `.claude/settings.json`

```json
{
  "env": {
    "ANTHROPIC_AUTH_TOKEN": "your-api-key-here",
    "ANTHROPIC_BASE_URL": "http://localhost:9880"
  }
}
```

**重要提示：**
- `ANTHROPIC_AUTH_TOKEN`：可以随意填写（本地部署无需真实密钥）
- `ANTHROPIC_BASE_URL`：本地部署使用 `http://localhost:9880`

#### 3.3 开始使用

配置完成后，Claude Code 将通过 maxx 代理访问 AI 服务。您可以在管理界面中查看使用情况和配额。

## 本地开发

### 服务器模式（浏览器）
后端：
```bash
go run cmd/maxx/main.go
```

前端：
```bash
cd web
npm install
npm run dev
```

### 桌面模式（Wails）
详细的桌面应用文档请参阅 `WAILS_README.md`。

快速开始：
```bash
# 安装 Wails CLI
go install github.com/wailsapp/wails/v2/cmd/wails@latest

# 运行桌面应用
wails dev

# 构建桌面应用
wails build
# 或
build-desktop.bat
```

## API 端点
- 管理 API: http://localhost:9880/admin/
- Web UI: http://localhost:9880/
- WebSocket: ws://localhost:9880/ws
- Claude: http://localhost:9880/v1/messages
- OpenAI: http://localhost:9880/v1/chat/completions
- Codex: http://localhost:9880/v1/responses
- Gemini: http://localhost:9880/v1beta/models/{model}:generateContent
- 项目代理: http://localhost:9880/{project-slug}/v1/messages (等)

## 数据存储
- 桌面模式（Windows）: `%APPDATA%\maxx`
- 服务器模式（非 Docker）: `~/.config/maxx/maxx.db`
- Docker 数据目录: `/data`（通过 `docker-compose.yml` 挂载）

## 发布版本

创建新版本发布有两种方式：

### GitHub Actions（推荐）

1. 进入仓库的 [Actions](../../actions) 页面
2. 选择 "Release" workflow
3. 点击 "Run workflow"
4. 输入版本号（如 `v1.0.0`）
5. 点击 "Run workflow" 执行

### 本地脚本

```bash
./release.sh <github_token> <version>
```

示例：
```bash
./release.sh ghp_xxxx v1.0.0
```

两种方式都会自动创建 tag 并生成 release notes。
