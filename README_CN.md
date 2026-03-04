<p align="center">
  <img src="web/public/logo.png" alt="maxx logo" width="128" height="128">
</p>

# maxx

[English](README.md) | 简体中文

多提供商 AI 代理服务，内置管理界面、路由和使用追踪功能。

## 预览

<p align="center">
  <img src="web/public/preview.png" alt="maxx dashboard preview" width="960">
</p>

## 功能特性

- **多协议代理**：支持 Claude、OpenAI、Gemini 和 Codex API 格式
- **AI 编程工具支持**：兼容 Claude Code、Codex CLI 等 AI 编程工具
- **供应商管理**：支持自定义中转站、Antigravity (Google)、Kiro (AWS) 供应商类型
- **智能路由**：优先级路由和加权随机路由策略
- **多数据库**：支持 SQLite（默认）、MySQL 和 PostgreSQL
- **使用追踪**：纳美元精度计费，支持请求倍率记录
- **模型定价**：版本化定价，支持分层定价和缓存价格
- **管理界面**：Web UI 支持多语言，WebSocket 实时更新
- **性能分析**：内置 pprof 支持，便于调试
- **备份恢复**：配置导入导出功能

## 快速开始

Maxx 支持三种部署方式：

| 方式 | 说明 | 适用场景 |
|------|------|----------|
| **Docker** | 容器化部署 | 服务器/生产环境 |
| **桌面应用** | 原生应用带 GUI | 个人使用 |
| **本地构建** | 从源码构建 | 开发环境 |

### Docker（服务器推荐）

```bash
docker compose up -d
```

服务将在 `http://localhost:9880` 上运行。

<details>
<summary>📄 完整的 docker-compose.yml 示例</summary>

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
      - MAXX_ADMIN_PASSWORD=your-password  # 可选：启用管理员认证
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

### 桌面应用（个人使用推荐）

从 [GitHub Releases](https://github.com/awsl-project/maxx/releases) 下载：

| 平台 | 文件 | 说明 |
|------|------|------|
| Windows | `maxx.exe` | 直接运行 |
| macOS (ARM) | `maxx-macOS-arm64.dmg` | Apple Silicon (M1/M2/M3/M4) |
| macOS (Intel) | `maxx-macOS-amd64.dmg` | Intel 芯片 |
| Linux | `maxx` | 原生二进制 |

<details>
<summary>🍺 macOS Homebrew 安装</summary>

```bash
# 安装
brew install --no-quarantine awsl-project/awsl/maxx

# 升级
brew upgrade --no-quarantine awsl-project/awsl/maxx
```

> **提示：** 如果提示"应用已损坏"，请运行：`sudo xattr -d com.apple.quarantine /Applications/maxx.app`

</details>

### 本地构建

```bash
# 服务器模式
go run cmd/maxx/main.go

# 启用管理员认证
MAXX_ADMIN_PASSWORD=your-password go run cmd/maxx/main.go

# 桌面模式 (Wails)
go install github.com/wailsapp/wails/v2/cmd/wails@latest
wails dev
```

## 配置 AI 编程工具

### Claude Code

在 maxx 管理界面中创建项目并生成 API 密钥。

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

<details>
<summary>🔧 Shell 函数（替代方案）</summary>

添加到你的 shell 配置文件（`~/.bashrc`、`~/.zshrc` 等）：

```bash
claude_maxx() {
    export ANTHROPIC_BASE_URL="http://localhost:9880"
    export ANTHROPIC_AUTH_TOKEN="your-api-key-here"
    claude "$@"
}
```

然后使用 `claude_maxx` 代替 `claude`。

</details>

<details>
<summary>🔐 Token 认证说明</summary>

**开启 Token 认证时：**
- 将 `ANTHROPIC_AUTH_TOKEN` 设置为在「API 令牌」页面创建的 Token（格式：`maxx_xxx`）
- Claude Code 会自动在请求头中添加 `x-api-key`
- maxx 会在处理请求前验证 Token

**关闭 Token 认证时：**
- 可以将 `ANTHROPIC_AUTH_TOKEN` 设置为任意值（如 `"dummy"`）或留空
- maxx 不会验证 Token
- 适用于内网环境或测试场景
- ⚠️ **警告：** 关闭 Token 认证会降低安全性

</details>

### Codex CLI

**config.toml**

在 `~/.codex/config.toml` 中添加：

```toml
# 可选：设置为默认 provider
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

创建或编辑 `~/.codex/auth.json`：

```json
{
  "OPENAI_API_KEY": "maxx_your_token_here"
}
```

**使用方法：**

```bash
# 使用 --provider 参数指定
codex --provider maxx

# 或者设置为默认 provider 后直接使用
codex
```

<details>
<summary>🔐 Token 认证说明</summary>

**开启 Token 认证时：**
- 在 `auth.json` 中配置 `OPENAI_API_KEY` 为在「API 令牌」页面创建的 Token（格式：`maxx_xxx`）
- Codex CLI 会自动在请求头中添加 `Authorization: Bearer <token>`
- maxx 会在处理请求前验证 Token

**关闭 Token 认证时：**
- 可以在 `auth.json` 中将 `OPENAI_API_KEY` 设置为任意值（如 `"dummy"`）
- maxx 不会验证 Token
- 适用于内网环境或测试场景
- ⚠️ **警告：** 关闭 Token 认证会降低安全性

</details>

## API 端点

| 类型 | 端点 |
|------|------|
| Claude | `POST /v1/messages` |
| OpenAI | `POST /v1/chat/completions` |
| Codex | `POST /v1/responses` |
| Gemini | `POST /v1beta/models/{model}:generateContent` |
| 项目代理 | `/{project-slug}/v1/messages` (等) |
| 管理 API | `/api/admin/*` |
| WebSocket | `ws://localhost:9880/ws` |
| 健康检查 | `GET /health` |
| Web UI | `http://localhost:9880/` |

## 配置说明

### 环境变量

| 变量 | 说明 |
|------|------|
| `MAXX_ADMIN_PASSWORD` | 启用管理员 JWT 认证。默认用户名：`admin`，密码为该变量的值 |
| `MAXX_DSN` | 数据库连接字符串 |
| `MAXX_DATA_DIR` | 自定义数据目录路径 |

### 系统设置

通过管理界面配置：

| 设置项 | 说明 | 默认值 |
|--------|------|--------|
| `proxy_port` | 代理服务器端口 | `9880` |
| `request_retention_hours` | 请求日志保留时间（小时） | `168`（7 天） |
| `request_detail_retention_seconds` | 请求详情保留时间（秒） | `-1`（永久） |
| `timezone` | 时区设置 | `Asia/Shanghai` |
| `quota_refresh_interval` | Antigravity 配额刷新间隔（分钟） | `0`（禁用） |
| `auto_sort_antigravity` | 自动排序 Antigravity 路由 | `false` |
| `enable_pprof` | 启用 pprof 性能分析 | `false` |
| `pprof_port` | pprof 服务端口 | `6060` |
| `pprof_password` | pprof 访问密码 | （空） |

### 数据库配置

Maxx 支持 SQLite（默认）、MySQL 和 PostgreSQL。

<details>
<summary>🗄️ MySQL 配置</summary>

```bash
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

</details>

<details>
<summary>🐘 PostgreSQL 配置</summary>

```bash
export MAXX_DSN="postgres://user:password@host:port/dbname?sslmode=disable"

# 示例
export MAXX_DSN="postgres://maxx:secret@127.0.0.1:5432/maxx?sslmode=disable"
```

</details>

### 数据存储位置

| 部署方式 | 位置 |
|----------|------|
| Docker | `/data`（挂载卷） |
| 桌面应用 (Windows) | `%USERPROFILE%\AppData\Local\maxx\` |
| 桌面应用 (macOS) | `~/Library/Application Support/maxx/` |
| 桌面应用 (Linux) | `~/.local/share/maxx/` |
| 服务器 (非 Docker) | `~/.config/maxx/maxx.db` |

## 本地开发

<details>
<summary>🛠️ 开发环境设置</summary>

### 国内镜像设置（中国大陆用户推荐）

```bash
# Go Modules Proxy
go env -w GOPROXY=https://goproxy.cn,direct

# pnpm Registry
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

详细文档请参阅 `WAILS_README.md`。

```bash
# 安装 Wails CLI
go install github.com/wailsapp/wails/v2/cmd/wails@latest

# 运行桌面应用
wails dev

# 构建桌面应用
wails build
```

</details>

## 发布版本

<details>
<summary>📦 发布流程</summary>

### GitHub Actions（推荐）

1. 进入仓库的 [Actions](../../actions) 页面
2. 选择 "Release" workflow
3. 点击 "Run workflow"
4. 输入版本号（如 `v1.0.0`）
5. 点击 "Run workflow" 执行

### 本地脚本

```bash
./release.sh <github_token> <version>

# 示例
./release.sh ghp_xxxx v1.0.0
```

两种方式都会自动创建 tag 并生成 release notes。

</details>

## 致谢

特别感谢 [router-for-me/CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI) 开源项目，为本项目在转发兼容性设计上提供了重要参考与启发。
