<p align="center">
  <img src="web/public/logo.png" alt="maxx logo" width="128" height="128">
</p>

# maxx

[English](README.md) | 简体中文

多提供商 AI 代理服务，内置管理界面、路由和使用追踪功能。

## 功能特性
- 支持 Claude、OpenAI、Gemini 和 Codex 格式的代理端点
- 兼容 Claude Code、Codex CLI 等 AI 编程工具，可作为统一的 API 代理网关
- 管理 API 和 Web UI
- 提供商路由、重试和配额管理
- 基于 SQLite 的数据存储

## 如何使用

Maxx 支持三种部署方式：

| 方式 | 说明 | 适用场景 |
|------|------|----------|
| **Docker** | 容器化部署 | 服务器/生产环境 |
| **桌面应用** | 原生应用带 GUI | 个人使用，简单易用 |
| **本地构建** | 从源码构建 | 开发环境 |

### 方式一：Docker（服务器推荐）

使用 Docker Compose 启动服务：

```bash
docker compose up -d
```

服务将在 `http://localhost:9880` 上运行。

<details>
<summary>完整的 docker-compose.yml 示例</summary>

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

### 方式二：桌面应用（个人使用推荐）

从 [GitHub Releases](https://github.com/awsl-project/maxx/releases) 下载预构建的桌面应用。

| 平台 | 文件 | 说明 |
|------|------|------|
| Windows | `maxx.exe` | 直接运行 |
| macOS (ARM) | `maxx-macOS-arm64.dmg` | Apple Silicon (M1/M2/M3) |
| macOS (Intel) | `maxx-macOS-amd64.dmg` | Intel 芯片 |
| Linux | `maxx` | 原生二进制 |

> **macOS 提示：** 如果提示"应用已损坏"，请运行：`sudo xattr -d com.apple.quarantine /Applications/maxx.app`

### 方式三：本地构建

```bash
# 运行服务器模式
go run cmd/maxx/main.go

# 或使用 Wails 运行桌面模式
go install github.com/wailsapp/wails/v2/cmd/wails@latest
wails dev
```

## 配置 AI 编程工具

### Claude Code

在 maxx 管理界面中创建项目并生成 API 密钥，然后使用以下方式之一配置 Claude Code：

**settings.json（推荐）**

配置位置：`~/.claude/settings.json` 或 `.claude/settings.json`

```json
{
  "env": {
    "ANTHROPIC_AUTH_TOKEN": "your-api-key-here",
    "ANTHROPIC_BASE_URL": "http://localhost:9880"
  }
}
```

**Shell 函数（替代方案）**

添加到你的 shell 配置文件（`~/.bashrc`、`~/.zshrc` 等）：

```bash
claude_maxx() {
    export ANTHROPIC_BASE_URL="http://localhost:9880"
    export ANTHROPIC_AUTH_TOKEN="your-api-key-here"
    claude "$@"
}
```

然后使用 `claude_maxx` 代替 `claude` 来通过 maxx 运行 Claude Code。

> **提示：** 本地部署时 `ANTHROPIC_AUTH_TOKEN` 可以随意填写。

### Codex CLI

在 `~/.codex/config.toml` 中添加以下配置：

```toml
[model_providers.maxx]
name = "maxx"
base_url = "http://localhost:9880"
wire_api = "responses"
request_max_retries = 4
stream_max_retries = 10
stream_idle_timeout_ms = 300000
```

然后在运行 Codex CLI 时使用 `--provider maxx` 参数。

## 本地开发

### 国内镜像设置（中国大陆用户推荐）

为了加速依赖下载，建议设置国内镜像源：

**Go Modules Proxy**
```bash
go env -w GOPROXY=https://goproxy.cn,direct
```

**pnpm Registry**
```bash
pnpm config set registry https://registry.npmmirror.com
```

### 服务器模式（浏览器）
**先构建前端：**
```bash
cd web
pnpm install
pnpm build
```

**然后运行后端：**
```bash
go run cmd/maxx/main.go
```

**或运行前端开发服务器（开发调试用）：**
```bash
cd web
pnpm dev
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

| 部署方式 | 数据位置 |
|----------|----------|
| Docker | `/data`（通过 volume 挂载） |
| 桌面应用 (Windows) | `%USERPROFILE%\AppData\Local\maxx\` |
| 桌面应用 (macOS) | `~/Library/Application Support/maxx/` |
| 桌面应用 (Linux) | `~/.local/share/maxx/` |
| 服务器 (非 Docker) | `~/.config/maxx/maxx.db` |

## 数据库配置

Maxx 支持 SQLite（默认）和 MySQL 数据库。

### SQLite（默认）

无需配置，数据存储在数据目录下的 `maxx.db` 文件中。

### MySQL

设置 `MAXX_DSN` 环境变量：

```bash
# MySQL DSN 格式
export MAXX_DSN="mysql://user:password@tcp(host:port)/dbname?parseTime=true&charset=utf8mb4"

# 示例
export MAXX_DSN="mysql://maxx:secret@tcp(127.0.0.1:3306)/maxx?parseTime=true&charset=utf8mb4"
```

**Docker Compose 使用 MySQL：**

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
