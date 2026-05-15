# Multi-instance e2e tests

Real Docker stack:**两个 maxx HTTP server 实例**共享一个 Redis 和一个 Postgres,
通过 admin API 端到端验证 multi-instance 协调行为。

是对 `tests/multiinstance/` 的真容器化补充:那一组用 miniredis 跑代码层语义,
这一组覆盖网络 + 真 Redis pub/sub + 共享 SQL + 跨进程 HTTP API。

## 跑法

```bash
cd tests/multiinstance-e2e

# 构建并启动 stack(maxx-a + maxx-b + redis + postgres + test-runner)
docker compose up --build --abort-on-container-exit --exit-code-from test-runner

# 清理
docker compose down -v
```

`--abort-on-container-exit` + `--exit-code-from test-runner`:test-runner 退出后
整个 stack 停下,exit code 反映测试结果(非零 = 有 case 失败)。

## 覆盖的场景

| 测试 | 验证什么 |
|---|---|
| `test_cache_invalidation_provider` | A 创建 provider → B 通过 cache invalidation pub/sub 看到 |
| `test_cooldown_cross_instance` | A `PUT /admin/cooldowns/{id}` → B 通过 generation 同步看到 |
| `test_both_instances_remain_healthy` | 两实例并存 8s,heartbeat 不打架,健康检查持续 OK |
| `test_rolling_data_integrity` | 在 A 上写 N provider,B 立即看到全部(无中间 stale 状态) |

## 配置

`docker-compose.yml` 把心跳/sweep 调短(`INSTANCE_TTL=10s` / `HEARTBEAT=3s` /
`SWEEP=5s`),让测试能在分钟级跑完。生产默认是 60s/20s/45s。

## 已知限制

- test-runner 没装 docker 客户端,不能 `kill -9 maxx-a`。真正的 hard-kill +
  60s grace sweep 由 `tests/multiinstance/sweep_test.go` 在代码层覆盖。
- 没跑负载/吞吐验证,只验证语义。
- 没验证 degraded 模式(Redis 启动失败 fallback 到 memory)— 因为 `fail-fast`
  模式下 Redis 必须 healthy,docker compose 用 `service_healthy` 等它。
