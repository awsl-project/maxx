# 5 分钟让 Claude/Codex 请求通过 maxx

这份文档给第一次试用 maxx 的人看：先跑通一条真实请求，再慢慢理解所有 tab。

完成后你会得到：

1. 一个正在运行的 maxx 服务；
2. 一个上游供应商；
3. 一条路由；
4. 一个 API token；
5. Claude Code 或 Codex CLI 通过 maxx 发出的第一条请求。

## 1. 启动 maxx

本地评估可以直接跑容器，并用 volume 保留数据：

```bash
docker run --rm -p 9880:9880 -v maxx-data:/data ghcr.io/awsl-project/maxx:latest
```

打开 `http://localhost:9880`。

如果要部署给多人使用，暴露服务前先设置管理员密码：

```bash
docker run -d \
  --name maxx \
  --restart unless-stopped \
  -p 9880:9880 \
  -v maxx-data:/data \
  -e MAXX_ADMIN_PASSWORD='change-me' \
  ghcr.io/awsl-project/maxx:latest
```

## 2. 添加供应商

在管理界面里：

1. 打开 **Providers / 供应商**。
2. 新增一个上游供应商。
3. 填入供应商 base URL、API key 和支持的客户端类型。
4. 保存。
5. 如果页面提供测试动作，先测试供应商是否可用。

如果上游是 OpenAI-compatible 服务，走 OpenAI-compatible / custom relay 配置路径，把供应商密钥留在 maxx 里，不要散落到每个本地工具配置中。

## 3. 创建路由

打开 **Routes / 路由**，按你最先要用的客户端协议创建路由：

- **Claude Code**：创建 Claude 路由。
- **Codex CLI**：创建 OpenAI Responses 路由。
- **通用 OpenAI 客户端**：创建 OpenAI 路由。

第一次只放一个供应商、一条路由。先跑通，再加故障转移和权重。

## 4. 创建 API token

打开 **API Tokens / API 令牌**，为本地工具或用户创建一个 token。

复制这个 token。它通常形如 `maxx_...`。

## 5. 配置本地工具

### Claude Code

写入 `~/.claude/settings.json` 或项目内 `.claude/settings.json`：

```json
{
  "env": {
    "ANTHROPIC_AUTH_TOKEN": "maxx_your_token_here",
    "ANTHROPIC_BASE_URL": "http://localhost:9880"
  }
}
```

然后启动 Claude Code，发一条小请求。

### Codex CLI

写入 `~/.codex/config.toml`：

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

把 maxx 里创建的 API token 作为 Codex 的 OpenAI auth token，然后发一条小请求。

## 6. 验证请求

回到 maxx，打开 **Requests / 请求**。

你应该能看到工具请求、命中的路由/供应商、状态、延迟和用量详情。如果这里没有请求，说明本地工具还没有真正指向 maxx。

## 常见问题

- **401 / unauthorized**：本地工具没有带 token，或带的不是 maxx 创建的 `maxx_...` API token。
- **404 / route not found**：没有为当前客户端实际使用的协议创建路由。
- **供应商错误**：先在 maxx 里测试供应商，再看请求详情。
- **Codex 长任务卡住**：保留示例里的 WebSocket 配置，并查看请求详情页。
- **请求页没有记录**：客户端仍在使用原供应商 URL，没有指向 `http://localhost:9880`。

## 跑通后再配置什么

第一条请求跑通后，再逐步加：

- 备用供应商；
- 加权路由；
- 模型价格；
- 用户 token 和额度；
- 请求保留策略；
- 供应商和路由配置备份。
