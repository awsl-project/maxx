#!/usr/bin/env bash
# 多实例 e2e 测试。两个 maxx HTTP server (maxx-a, maxx-b) 共享 redis + postgres,
# 这个脚本通过 admin API 验证跨实例的协调行为是否符合 docs/multi-instance-rfc.md。
#
# 每个测试:
#   1) 通过 HTTP 改一个实例的状态(provider / cooldown 等)
#   2) 通过 HTTP 查另一个实例,断言它看到了新状态
#
# 失败时 set -e 让脚本立刻停下来,test-runner 容器 exit code != 0 → compose 退出非零。

set -euo pipefail

MAXX_A="${MAXX_A_URL:?MAXX_A_URL required}"
MAXX_B="${MAXX_B_URL:?MAXX_B_URL required}"
PASS="${MAXX_ADMIN_PASSWORD:?MAXX_ADMIN_PASSWORD required}"

PASSED=0
FAILED=0

# ---------- helpers ----------

# login <url> → echo JWT
login() {
  local url="$1"
  local resp token
  resp=$(curl -sf "$url/api/admin/auth/login" \
    -H "Content-Type: application/json" \
    -d "{\"username\":\"admin\",\"password\":\"$PASS\"}")
  token=$(echo "$resp" | jq -r '.token // empty')
  if [ -z "$token" ]; then
    echo "FAIL: login at $url" >&2
    echo "$resp" >&2
    return 1
  fi
  echo "$token"
}

# api <method> <url> <token> [<body>] → response body, exits non-zero on HTTP error
api() {
  local method="$1" url="$2" token="$3" body="${4:-}"
  if [ -z "$body" ]; then
    curl -sf -X "$method" "$url" -H "Authorization: Bearer $token"
  else
    curl -sf -X "$method" "$url" \
      -H "Authorization: Bearer $token" \
      -H "Content-Type: application/json" \
      -d "$body"
  fi
}

# wait_for <timeout_sec> <cmd...> — poll until cmd returns 0 or timeout.
#
# 注意:cmd 通过 "$@" 透传,**不要**包成 `bash -c "..."` —— 子 shell 拿不到
# parent shell 里定义的 function(如 `api`),会以 command-not-found 静默失败。
# 调用方应该把闭包逻辑写在一个 wrapper function 里然后传函数名,或者用
# `helper_xxx_check` 命名约定,本脚本里有具体例子。
wait_for() {
  local timeout="$1"; shift
  local deadline=$(($(date +%s) + timeout))
  while [ "$(date +%s)" -lt "$deadline" ]; do
    if "$@"; then
      return 0
    fi
    sleep 0.5
  done
  return 1
}

# 提供给 wait_for 用的 check helpers。不能用 bash -c 内联表达式,
# 因为 subshell 拿不到 api()。
check_provider_visible_on_b() {
  local pid="$1" token="$2"
  api GET "$MAXX_B/api/admin/providers/$pid" "$token" >/dev/null 2>&1
}

check_cooldown_visible_on_b() {
  local pid="$1" token="$2"
  local cd
  cd=$(api GET "$MAXX_B/api/admin/cooldowns" "$token" 2>/dev/null) || return 1
  echo "$cd" | jq -e --argjson pid "$pid" '.[] | select(.providerID == $pid)' >/dev/null 2>&1
}

check_providers_count_matches() {
  local expected="$1" tok_a="$2" tok_b="$3"
  local a b
  a=$(api GET "$MAXX_A/api/admin/providers" "$tok_a" 2>/dev/null | jq 'length' 2>/dev/null) || return 1
  b=$(api GET "$MAXX_B/api/admin/providers" "$tok_b" 2>/dev/null | jq 'length' 2>/dev/null) || return 1
  [ "$a" -eq "$expected" ] && [ "$b" -eq "$expected" ]
}

mark_pass() {
  PASSED=$((PASSED + 1))
  echo "PASS: $1"
}

mark_fail() {
  FAILED=$((FAILED + 1))
  echo "FAIL: $1" >&2
}

# ---------- setup ----------

echo "=== Multi-instance e2e: 2 maxx instances + shared Redis + shared Postgres ==="

echo "--- Logging in to both instances ---"
TOKEN_A=$(login "$MAXX_A")
TOKEN_B=$(login "$MAXX_B")
echo "OK: got tokens from both instances"

# ---------- Test 1: cache invalidation (provider) ----------
#
# Create a provider on A. B should see it via cache invalidation pub/sub.

test_cache_invalidation_provider() {
  local name="cache-inv-$(date +%s)"
  local body
  body=$(jq -nc --arg name "$name" \
    '{type:"custom",name:$name,config:{custom:{baseURL:"http://example.invalid",apiKey:"x"}},supportedClientTypes:["claude"]}')

  local created
  created=$(api POST "$MAXX_A/api/admin/providers" "$TOKEN_A" "$body")
  local pid
  pid=$(echo "$created" | jq -r '.id')
  if [ -z "$pid" ] || [ "$pid" = "null" ]; then
    mark_fail "test_cache_invalidation_provider: A didn't return id; resp=$created"
    return 1
  fi

  # B 应该在 cache invalidation pub/sub 到达后看到这个 provider
  if wait_for 10 check_provider_visible_on_b "$pid" "$TOKEN_B"; then
    mark_pass "test_cache_invalidation_provider (id=$pid visible on B)"
  else
    mark_fail "test_cache_invalidation_provider: B never saw provider $pid created on A"
  fi
}

# ---------- Test 2: cooldown 跨实例 ----------
#
# Admin 在 A 上把一个 provider 的 cooldown 设到未来,B 调 GET /api/admin/cooldowns 应当看到。

test_cooldown_cross_instance() {
  # 先在 A 上创建一个 provider 给 cooldown 用
  local body
  body=$(jq -nc \
    '{type:"custom",name:"cooldown-target",config:{custom:{baseURL:"http://example.invalid",apiKey:"x"}},supportedClientTypes:["claude"]}')
  local created pid
  created=$(api POST "$MAXX_A/api/admin/providers" "$TOKEN_A" "$body")
  pid=$(echo "$created" | jq -r '.id')

  # 等 B 也看到该 provider(cache invalidation 已经在 test 1 验证过)
  if ! wait_for 10 check_provider_visible_on_b "$pid" "$TOKEN_B"; then
    mark_fail "test_cooldown_cross_instance: B never saw provider $pid"
    return 1
  fi

  # A 上 PUT cooldown
  local until
  until=$(date -u -d '+10 minutes' +%FT%TZ 2>/dev/null || date -u -v+10M +%FT%TZ)
  local cd_body
  cd_body=$(jq -nc --arg t "$until" '{untilTime:$t,clientType:"",model:""}')
  api PUT "$MAXX_A/api/admin/cooldowns/$pid" "$TOKEN_A" "$cd_body" >/dev/null

  # B 应该通过 cooldown event/generation 同步看到
  if wait_for 10 check_cooldown_visible_on_b "$pid" "$TOKEN_B"; then
    mark_pass "test_cooldown_cross_instance (cooldown for provider=$pid visible on B)"
  else
    mark_fail "test_cooldown_cross_instance: B never saw the cooldown for provider $pid"
  fi
}

# ---------- Test 3: 实例 unregister 后,heartbeat 过期 → 健康实例继续工作 ----------
#
# 验证 instance heartbeat 机制:让 B 通过 admin API 主动 unregister(或等 A 关掉),
# 然后看 A 的 /health 仍然 OK,且 cache/cooldown 还能正常用。
#
# 注:test-runner 容器没有 docker 客户端,不能直接 kill -9 maxx-a。但我们可以让
# A 走优雅 shutdown 路径(通过 /api/admin/control/shutdown 或类似),或者断开
# 它的 redis 连接。这里我们只做"两实例同时存活时持续可用 + heartbeat 持续刷新"
# 这个最低保证;真正的 kill -9 + 接管 由 tests/multiinstance/sweep_test.go 在
# 代码层覆盖。

test_both_instances_remain_healthy() {
  for i in 1 2 3 4; do
    if ! curl -sf "$MAXX_A/health" >/dev/null; then
      mark_fail "test_both_instances_remain_healthy: A unhealthy on iter $i"
      return 1
    fi
    if ! curl -sf "$MAXX_B/health" >/dev/null; then
      mark_fail "test_both_instances_remain_healthy: B unhealthy on iter $i"
      return 1
    fi
    sleep 2
  done
  mark_pass "test_both_instances_remain_healthy (both /health OK over 8s)"
}

# ---------- Test 4: 滚动重启数据完整性 ----------
#
# 在 A 上创建一系列 provider,模拟"持续写入"。在 B 上 GET 列表,
# 验证 list 长度一致(允许小延迟)。这是最简的 rolling-update 数据完整性 smoke。

test_rolling_data_integrity() {
  local before
  before=$(api GET "$MAXX_A/api/admin/providers" "$TOKEN_A" | jq 'length')

  for i in 1 2 3; do
    local body
    body=$(jq -nc --arg n "roll-$i-$(date +%s%N)" \
      '{type:"custom",name:$n,config:{custom:{baseURL:"http://example.invalid",apiKey:"x"}},supportedClientTypes:["claude"]}')
    api POST "$MAXX_A/api/admin/providers" "$TOKEN_A" "$body" >/dev/null
  done

  local expected=$((before + 3))
  if wait_for 10 check_providers_count_matches "$expected" "$TOKEN_A" "$TOKEN_B"; then
    mark_pass "test_rolling_data_integrity (both A and B see all 3 new providers)"
  else
    local a_now b_now
    a_now=$(api GET "$MAXX_A/api/admin/providers" "$TOKEN_A" | jq 'length' 2>/dev/null || echo "?")
    b_now=$(api GET "$MAXX_B/api/admin/providers" "$TOKEN_B" | jq 'length' 2>/dev/null || echo "?")
    mark_fail "test_rolling_data_integrity: counts diverged. before=$before, after expected=$((before + 3)), A=$a_now, B=$b_now"
  fi
}

# ---------- run ----------

test_cache_invalidation_provider
test_cooldown_cross_instance
test_both_instances_remain_healthy
test_rolling_data_integrity

echo ""
echo "=== Results: $PASSED passed, $FAILED failed ==="
if [ "$FAILED" -gt 0 ]; then
  exit 1
fi
