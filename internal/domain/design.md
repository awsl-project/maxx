# Maxx-Next 设计文档

## 概述

一个高性能的 AI API 代理网关，支持多种客户端类型和多个供应商。

---

## 核心流程

```
Request
  ↓
ClientAdapter.Match()        → 确定 ClientType
ClientAdapter.ExtractInfo()  → 提取 SessionID, RequestModel
  ↓
ctx 写入 ClientType, SessionID, RequestModel
  ↓
Router 根据 RoutingStrategy 匹配 Route 列表
  ↓
遍历 Route:
  ├── 计算 MappedModel (Route > Provider > 原始)
  ├── ctx 写入 MappedModel
  ├── ProviderAdapter.Execute()
  ├── err == nil → 成功，跳出
  ├── 未写入客户端 + 失败 → 按 RetryConfig 重试 / 下一个 Route
  └── 已写入客户端 + 失败 → 直接整体失败，跳出
  ↓
成功: Execute 过程中将 ResponseModel 写入 ctx
  ↓
Response
```

---

## 组件设计

### 1. ClientAdapter（识别层）

每种 ClientType 一个，职责：
- 识别请求是否属于该 ClientType
- 提取 SessionID、RequestModel 等信息

```go
type ClientAdapter interface {
    // 判断请求是否属于该 ClientType
    Match(req *http.Request) bool

    // 提取请求信息
    ExtractInfo(req *http.Request) (*ClientRequestInfo, error)
}

type ClientRequestInfo struct {
    SessionID    string
    RequestModel string
}
```

### 2. ProviderAdapter（执行层）

按 Provider 分目录，每个目录下按 ClientType 实现：

```
adapters/
├── custom/
│   ├── claude.go
│   ├── openai.go
│   ├── gemini.go
│   └── codex.go
└── antigravity/
    ├── claude.go
    └── openai.go
```

职责：
- 请求转换
- 执行请求（含流式）
- 响应处理
- 失败判定
- 过程中将 ResponseModel 写入 ctx

```go
type ProviderAdapter interface {
    // 支持的 ClientType 列表
    SupportedClientTypes() []ClientType

    // 执行代理请求
    // 内部根据 ClientType 分发到具体实现
    // 成功时将 ResponseModel 写入 ctx
    // 失败返回 ProxyError
    Execute(ctx context.Context, w http.ResponseWriter, req *http.Request) error
}
```

Provider 内部实现示例：

```go
type CustomProvider struct {
    config   *ProviderConfigCustom
    handlers map[ClientType]ClientHandler
}

func (p *CustomProvider) Execute(ctx context.Context, w http.ResponseWriter, req *http.Request) error {
    clientType := GetClientType(ctx)
    handler := p.handlers[clientType]
    return handler.Handle(ctx, w, req)
}
```

### 3. 全局注册

只到 ProviderType 级别，Provider 内部自己注册 ClientType：

```go
var providerAdapters = map[ProviderType]NewProviderAdapterFunc{
    "custom":      NewCustomProviderAdapter,
    "antigravity": NewAntigravityProviderAdapter,
}
```

---

## 失败与重试

### 错误类型

```go
type ProxyError struct {
    Err       error
    Retryable bool  // 是否可重试
}
```

### 判定标准

| 状态 | Retryable |
|-----|-----------|
| 未开始写入客户端 | true |
| 已开始写入客户端 | false |

失败条件：
- HTTP 非 2xx
- 超时
- Body 中特定错误（由 Adapter 判断）
- 流式/响应中断

### 重试逻辑

```
遍历 Route:
  ├── 执行 Execute
  ├── 成功 → 跳出
  ├── Retryable + 未超过 MaxRetries → 重试当前 Route
  ├── Retryable + 超过 MaxRetries → 下一个 Route
  └── 不可重试 → 整体失败
```

---

## 配置查找逻辑

### RetryConfig 查找

```
Route.RetryConfigID != 0  → 使用指定配置
Route.RetryConfigID == 0  → 使用系统默认配置 (IsDefault = true)
```

### RoutingStrategy 查找

```
ProjectID 有对应策略  → 使用 Project 策略
ProjectID 无对应策略  → 使用全局策略 (ProjectID = 0)
```

### Model 映射查找

```
Route.ModelMapping[requestModel] 存在    → 使用 Route 映射
Provider.ModelMapping[requestModel] 存在 → 使用 Provider 映射
都不存在 → 使用原始 RequestModel
```

---

## Model 三层

| 层级 | 说明 |
|-----|------|
| RequestModel | 客户端请求的 Model |
| MappedModel | Provider/Route 映射后的 Model |
| ResponseModel | 上游实际返回的 Model |

示例：
```
Client 请求 "claude-3-opus"      (RequestModel)
    ↓
映射为 "anthropic/claude-3-opus"  (MappedModel)
    ↓
上游返回 "claude-3-opus-20240229" (ResponseModel)
```

---

## Context 传递

通过独立 key 存取，不打包成结构体：

```go
type contextKey string

const (
    CtxKeyClientType    contextKey = "client_type"
    CtxKeySessionID     contextKey = "session_id"
    CtxKeyProjectID     contextKey = "project_id"
    CtxKeyRequestModel  contextKey = "request_model"
    CtxKeyMappedModel   contextKey = "mapped_model"
    CtxKeyResponseModel contextKey = "response_model"
)
```

---

## Router 设计

### 内存数据管理

所有配置数据常驻内存（单实例部署）：
- Provider
- Route
- RoutingStrategy
- RetryConfig

启动时加载，通过 API 修改时直接更新内存。

### 数据结构

```go
// Router 匹配结果，预关联所有需要的数据
type MatchedRoute struct {
    Route       *Route
    Provider    *Provider
    RetryConfig *RetryConfig  // 已解析，包括默认配置
}

type Router struct {
    // 内存数据
    routes             []*Route
    routingStrategies  []*RoutingStrategy
    providers          map[uint64]*Provider
    retryConfigs       map[uint64]*RetryConfig
    defaultRetryConfig *RetryConfig
}
```

### 接口

```go
func (r *Router) Match(clientType ClientType, projectID uint64) ([]*MatchedRoute, error)
```

### Match 逻辑

```
1. 筛选 Route
   - 条件: IsEnabled && ClientType 匹配
   - Project 优先: 先查 ProjectID == 请求的 ProjectID
   - 没有则用全局: ProjectID == 0

2. 获取 RoutingStrategy
   - Project 优先: 先查 ProjectID == 请求的 ProjectID
   - 没有则用全局: ProjectID == 0

3. 按策略排序
   - priority: 按 Position 升序
   - weighted_random: 按权重随机排列

4. 组装 MatchedRoute
   - 关联 Provider (by Route.ProviderID)
   - 关联 RetryConfig (Route.RetryConfigID，0 则用默认)

5. 返回列表
   - 空列表返回 error
```

---

## 可插拔中间件

预留位置，之后可插入：
- 限流
- 日志
- 指标
- 认证

```
Request
  ↓
[Middleware Chain]  ← 可插拔
  ↓
ClientAdapter
  ↓
Router
  ↓
Executor
  ↓
Response
```
