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
  <a href="README.md">English</a> | 简体中文
</p>

<p align="center">
  多提供商 AI 代理服务，内置管理界面、路由和使用追踪功能。
</p>

<p align="center">
  <a href="docs/database-migrations.md">文档</a> · <a href="https://github.com/awsl-project/maxx/releases">发行版</a> · <a href="https://ghcr.io/awsl-project/maxx">Docker 镜像</a>
</p>

## 功能特性

- **协议兼容**：支持 Claude、OpenAI、Gemini、Codex API 格式
- **AI 工具友好**：兼容 Claude Code、Codex CLI 等 AI 编程工具
- **供应商类型**：自定义中转站、Antigravity (Google)、Kiro (AWS)
- **路由策略**：优先级路由、加权随机路由
- **数据库**：SQLite（默认）、MySQL、PostgreSQL
- **用量与计费**：请求记录 + 纳美元精度计费，支持倍率
- **模型定价**：版本化定价，支持分层与缓存价格
- **管理界面**：多语言 Web UI，WebSocket 实时更新
- **性能分析**：内置 pprof
- **备份恢复**：配置导入/导出

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
brew install --cask awsl-project/awsl/maxx

# 升级
brew upgrade --cask awsl-project/awsl/maxx
```

> **Gatekeeper 提示：** `maxx` 尚未经过 notarization，首次启动时 macOS Gatekeeper 可能会阻止打开。
> 可执行：
> `xattr -d com.apple.quarantine /Applications/maxx.app`
>
> 或前往 **系统设置 > 隐私与安全性**，点击 **仍要打开**。
>
> **如果 macOS 提示“应用已损坏”：**
> 1. 清理隔离属性：
>    `sudo xattr -rd com.apple.quarantine /Applications/maxx.app`
> 2. 在访达中右键 `maxx.app`，选择一次**打开**。
> 3. 如果仍失败，重装后再重试：
>    `brew uninstall --cask awsl-project/awsl/maxx && brew install --cask awsl-project/awsl/maxx`

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

**前端开发环境要求：** Node.js 22.12.0+（22.x 线内，见 `.node-version` / `.nvmrc`）以及 pnpm 10.7.0（在 `web/package.json` 中锁定）。

### 命令行管理工具（`maxx-cli`）

`maxx-cli` 是一个独立的二进制，通过 maxx server 的 admin HTTP API 完成 Web UI
能做的所有配置：providers、API tokens、routes（含 weight）、routing
strategies（含 sticky 会话亲和）、users、invite codes、settings。面向脚本、
CI 和 AI agent。

支持的安装方式：

```bash
# 从源码（最新 release tag）：
go install github.com/awsl-project/maxx/cmd/maxx-cli@latest

# 从本地仓库：
task install:cli      # 通过 Taskfile 安装到 $GOBIN

# 官方 Docker 镜像里已预装：/usr/local/bin/maxx-cli
docker exec <container> maxx-cli --help
```

独立 binary release 资产、Homebrew/Scoop 发布暂未提供，跟进
[#585](https://github.com/awsl-project/maxx/pull/585)。

首次使用：

```bash
maxx-cli login --server http://localhost:9880 --username admin
maxx-cli help reference       # 完整命令树，自动生成
maxx-cli -o json provider list
```

完整的 agent 友好简报：`maxx-cli help reference`、`maxx-cli help formatting`、
`maxx-cli help auth-config`。

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
| 项目代理 | `/project/{project-slug}/v1/messages` (等) |
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
| `MAXX_DISABLE_UI` | 无前端模式：取值为真（`1`/`true`/`yes`/`on`）时不再提供 Web UI，仅暴露 API 与代理接口。等价于 `-no-ui` 命令行参数（同时设置时以 flag 为准）。项目代理路由（`/project/{slug}/...`）仍然可用 |
| `MAXX_CORS_ALLOW_ORIGINS` | 允许跨源访问的来源列表（逗号分隔，或 `*`）。用于让单独托管的前端连接此后端；未设置时关闭 CORS（仅同源） |
| `MAXX_ROUTING_SEED_SALT` | 可选共享密钥，用于 `weighted_random` 路由策略。未设置时每个进程会生成自己的随机盐——防 SessionID 枚举仍然成立，Redis sticky 绑定也会在首次成功后跨实例收敛；但相同 `(token, session)` 在 sticky 写入前的首选顺序在各实例间可能不一致。**多实例部署且需要一致首选顺序**时，请在所有实例上设置相同的值 |

### 无前端模式（仅 API，不提供 Web UI）

把 maxx 作为纯 API 网关运行，不再提供管理 Web UI——适合服务端/生产部署：所有配置都通过 Admin API 完成，同时减小攻击面。

用 `-no-ui` 命令行参数**或** `MAXX_DISABLE_UI` 环境变量开启（两者同时设置时以 flag 为准）：

```bash
# 命令行参数（本地构建）
maxx -no-ui

# 环境变量（Docker / compose）
docker run -e MAXX_DISABLE_UI=true -p 9880:9880 ghcr.io/awsl-project/maxx
```

无前端模式下：

- `/` 及所有 Web UI 路由返回 `404`（不提供静态文件）。
- API（`/api/admin/*`）、代理接口（`/v1/messages`、`/v1/chat/completions` 等）、项目代理（`/project/{slug}/...`）、`/health`、`/ws` 均照常工作。
- 通过 Admin API 配置 provider、路由、token 等。建议设置 `MAXX_ADMIN_PASSWORD` 保护接口。

### 前端单独托管（让 UI 连接远程后端）

可以把 Web UI 托管在一个来源（如 CDN、开发服务器），让它连接**另一个来源**上的后端。

**1. 在后端放行前端来源**（CORS，否则浏览器会拦截跨源请求）：

```bash
# 单个来源
MAXX_CORS_ALLOW_ORIGINS=https://ui.example.com maxx

# 多个来源（逗号分隔），或用 "*" 放行任意来源
MAXX_CORS_ALLOW_ORIGINS=https://ui.example.com,http://localhost:3000 maxx
```

> ⚠️ **CORS 不能替代鉴权。** `*` 会让*任意*网站都能从浏览器读取并调用你的 API（包括管理 API）。只在受信/本地环境用 `*`，并务必设置 `MAXX_ADMIN_PASSWORD` 让管理 API 需要 token。尽量列举明确来源而非 `*`。当 `*` 与未鉴权的管理 API 同时出现时，maxx 启动时会打印告警。

**2. 让 UI 指向后端。** 打开 Web UI，二选一：

- 在**登录页**展开 **连接设置**，填入后端地址（如 `https://api.example.com`）；或
- 登录后进入 **设置 → 后端地址**。

该值保存在浏览器（`localStorage`），所以不同用户/浏览器可以连接不同后端。留空则使用提供页面的同源地址（默认）。构建期默认值：构建前端时设置 `VITE_BACKEND_URL`。

### 系统设置

通过管理界面配置：

| 设置项 | 说明 | 默认值 |
|--------|------|--------|
| `proxy_port` | 代理服务器端口 | `9880` |
| `request_retention_hours` | 请求日志保留时间（小时） | `168`（7 天） |
| `request_detail_retention_seconds` | 请求详情保留时间（秒，统一配置——split 关闭时生效） | `-1`（永久） |
| `request_detail_retention_split_enabled` | 是否分别配置成功/失败保留时长 | `false` |
| `request_detail_retention_seconds_success` | 成功请求详情保留时间（秒，仅 split=true 生效） | 未设置回退到统一键 |
| `request_detail_retention_seconds_failed` | 失败请求详情保留时间（秒，仅 split=true 生效） | 未设置回退到统一键 |
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
