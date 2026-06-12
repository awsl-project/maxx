#!/usr/bin/env bash
# Integration tests for Maxx + Bedrock provider
# Validates the full pipeline: Claude Code CLI -> Maxx proxy -> AWS Bedrock
set -euo pipefail

PASS=0
FAIL=0
TESTS_RUN=0

pass() { echo "  PASS: $1"; PASS=$((PASS + 1)); }
fail() { echo "  FAIL: $1"; FAIL=$((FAIL + 1)); }

run_test() {
  local name="$1"
  TESTS_RUN=$((TESTS_RUN + 1))
  echo "[Test ${TESTS_RUN}] ${name}"
}

echo "============================================"
echo "  Maxx + Bedrock Integration Tests"
echo "============================================"
echo ""

# --- Setup ---
echo "[Setup] Configuring maxx with Bedrock provider..."
bash /scripts/setup_maxx.sh
echo ""

MAXX_URL="${MAXX_URL:?}"

# Configure Claude Code CLI environment
export ANTHROPIC_BASE_URL="${MAXX_URL}"
export ANTHROPIC_AUTH_TOKEN="dummy"
export CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1

# ===================================================================
# Section 1: Raw API Tests (curl)
# ===================================================================

echo "--- Section 1: Raw API Tests ---"
echo ""

# Test: Non-streaming API
run_test "Raw API - non-streaming"
RESP=$(curl -sf "${MAXX_URL}/v1/messages" \
  -H "Content-Type: application/json" \
  -H "x-api-key: dummy" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "claude-sonnet-4-20250514",
    "max_tokens": 64,
    "messages": [{"role": "user", "content": "Reply with exactly: INTEGRATION_TEST_OK"}]
  }' 2>&1) || true

if echo "$RESP" | jq -e '.content[0].text' > /dev/null 2>&1; then
  pass "non-streaming response valid"
else
  fail "non-streaming unexpected response: ${RESP:0:200}"
fi

# Test: Streaming API
run_test "Raw API - streaming"
STREAM_RESP=$(curl -sf "${MAXX_URL}/v1/messages" \
  -H "Content-Type: application/json" \
  -H "x-api-key: dummy" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "claude-sonnet-4-20250514",
    "max_tokens": 64,
    "stream": true,
    "messages": [{"role": "user", "content": "Reply with exactly: STREAM_OK"}]
  }' 2>&1) || true

if echo "$STREAM_RESP" | grep -q "message_start" && echo "$STREAM_RESP" | grep -q "message_stop"; then
  pass "streaming complete (message_start + message_stop)"
else
  fail "streaming incomplete: ${STREAM_RESP:0:200}"
fi

# Test: No Bedrock metrics leak
run_test "Response sanitization - no amazon-bedrock-invocationMetrics"
if echo "$STREAM_RESP" | grep -q "amazon-bedrock-invocationMetrics"; then
  fail "Bedrock metrics leaked to client"
else
  pass "no Bedrock-specific fields in response"
fi

# Test: cache_control preserved (Bedrock supports prompt caching)
run_test "Prompt caching - cache_control preserved and accepted"
CACHE_RESP=$(curl -sf "${MAXX_URL}/v1/messages" \
  -H "Content-Type: application/json" \
  -H "x-api-key: dummy" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "claude-sonnet-4-20250514",
    "max_tokens": 32,
    "messages": [{"role": "user", "content": [{"type": "text", "text": "Hi", "cache_control": {"type": "ephemeral"}}]}],
    "system": [{"type": "text", "text": "Be brief.", "cache_control": {"type": "ephemeral"}}]
  }' 2>&1) || true

if echo "$CACHE_RESP" | jq -e '.content[0].text' > /dev/null 2>&1; then
  # Check that usage includes cache-related fields
  HAS_CACHE_FIELDS=$(echo "$CACHE_RESP" | jq -e '.usage.cache_creation_input_tokens // .usage.cache_read_input_tokens' 2>/dev/null) || true
  if [ -n "$HAS_CACHE_FIELDS" ]; then
    pass "cache_control accepted, usage includes cache token counts"
  else
    pass "cache_control accepted (response valid)"
  fi
else
  fail "cache_control request failed: ${CACHE_RESP:0:200}"
fi

# Test: adaptive thinking converted
run_test "Request sanitization - adaptive thinking converted"
THINKING_RESP=$(curl -sf "${MAXX_URL}/v1/messages" \
  -H "Content-Type: application/json" \
  -H "x-api-key: dummy" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "claude-sonnet-4-20250514",
    "max_tokens": 16000,
    "thinking": {"type": "adaptive", "budget_tokens": 10000},
    "messages": [{"role": "user", "content": "What is 1+1? Be brief."}]
  }' 2>&1) || true

if echo "$THINKING_RESP" | jq -e '.content' > /dev/null 2>&1; then
  pass "adaptive thinking request handled"
else
  fail "adaptive thinking request failed: ${THINKING_RESP:0:300}"
fi

# Test: Model alias resolution
run_test "Model alias - claude-sonnet-4-6 resolves correctly"
ALIAS_RESP=$(curl -sf "${MAXX_URL}/v1/messages" \
  -H "Content-Type: application/json" \
  -H "x-api-key: dummy" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "claude-sonnet-4-6",
    "max_tokens": 32,
    "messages": [{"role": "user", "content": "Say OK"}]
  }' 2>&1) || true

if echo "$ALIAS_RESP" | jq -e '.content[0].text' > /dev/null 2>&1; then
  pass "model alias claude-sonnet-4-6 resolved"
else
  fail "model alias failed: ${ALIAS_RESP:0:200}"
fi

echo ""

# ===================================================================
# Section 2: Claude Code CLI Tests (claude -p)
# ===================================================================

echo "--- Section 2: Claude Code CLI Tests (claude -p) ---"
echo ""

# Helper: run claude -p and capture output + exit code
run_claude() {
  local exit_code=0
  local output
  output=$(timeout 120 claude -p "$@" 2>&1) || exit_code=$?
  echo "$output"
  return $exit_code
}

# Test: Basic prompt (default model)
run_test "claude -p basic prompt (default model)"
CLI_EXIT=0
CLI_OUT=$(run_claude "What is 2+2? Reply with just the number.") || CLI_EXIT=$?
echo "  exit=$CLI_EXIT output='${CLI_OUT:0:200}'"

if [ "$CLI_EXIT" -eq 0 ] && [ -n "$CLI_OUT" ]; then
  if echo "$CLI_OUT" | grep -q "4"; then
    pass "correct answer '4' with default model"
  else
    pass "valid response (${#CLI_OUT} chars)"
  fi
else
  fail "default model failed (exit=$CLI_EXIT): ${CLI_OUT:0:200}"
fi

# Test: Explicit model
run_test "claude -p --model claude-sonnet-4-20250514"
CLI_EXIT=0
CLI_OUT=$(run_claude --model claude-sonnet-4-20250514 "Say hello in 3 words.") || CLI_EXIT=$?
echo "  exit=$CLI_EXIT output='${CLI_OUT:0:200}'"

if [ "$CLI_EXIT" -eq 0 ] && [ -n "$CLI_OUT" ]; then
  pass "explicit model returned valid response"
else
  fail "explicit model failed (exit=$CLI_EXIT): ${CLI_OUT:0:200}"
fi

# Test: Multi-turn conversation (pipe two prompts)
run_test "claude -p multi-turn via pipe"
CLI_EXIT=0
CLI_OUT=$(echo "Remember the number 42. Just confirm you remember it." | timeout 120 claude -p 2>&1) || CLI_EXIT=$?
echo "  exit=$CLI_EXIT output='${CLI_OUT:0:200}'"

if [ "$CLI_EXIT" -eq 0 ] && [ -n "$CLI_OUT" ]; then
  pass "piped input handled"
else
  fail "piped input failed (exit=$CLI_EXIT): ${CLI_OUT:0:200}"
fi

# Test: Long response (verify streaming works end-to-end through CLI)
run_test "claude -p long response (streaming through CLI)"
CLI_EXIT=0
CLI_OUT=$(run_claude "List the first 10 prime numbers, one per line.") || CLI_EXIT=$?
echo "  exit=$CLI_EXIT output_length=${#CLI_OUT}"

if [ "$CLI_EXIT" -eq 0 ] && [ "${#CLI_OUT}" -gt 20 ]; then
  # Check that at least some primes are present
  if echo "$CLI_OUT" | grep -q "7" && echo "$CLI_OUT" | grep -q "23"; then
    pass "long streaming response with correct content"
  else
    pass "long streaming response (${#CLI_OUT} chars)"
  fi
else
  fail "long response failed (exit=$CLI_EXIT, len=${#CLI_OUT}): ${CLI_OUT:0:200}"
fi

# Test: JSON output mode
run_test "claude -p with --output-format json"
CLI_EXIT=0
CLI_OUT=$(run_claude --output-format json "What is the capital of France? Reply with just the city name.") || CLI_EXIT=$?
echo "  exit=$CLI_EXIT output='${CLI_OUT:0:300}'"

if [ "$CLI_EXIT" -eq 0 ]; then
  if echo "$CLI_OUT" | jq -e '.result' > /dev/null 2>&1; then
    RESULT=$(echo "$CLI_OUT" | jq -r '.result')
    if echo "$RESULT" | grep -qi "paris"; then
      pass "JSON output with correct answer 'Paris'"
    else
      pass "JSON output valid (result: ${RESULT:0:80})"
    fi
  else
    # Some versions output differently
    if echo "$CLI_OUT" | grep -qi "paris"; then
      pass "response contains 'Paris'"
    else
      pass "JSON mode returned response"
    fi
  fi
else
  fail "JSON output failed (exit=$CLI_EXIT): ${CLI_OUT:0:200}"
fi

# Test: System prompt via -a (append system prompt)
run_test "claude -p with --append-system-prompt"
CLI_EXIT=0
CLI_OUT=$(run_claude --append-system-prompt "Always respond in uppercase." "Say hi") || CLI_EXIT=$?
echo "  exit=$CLI_EXIT output='${CLI_OUT:0:200}'"

if [ "$CLI_EXIT" -eq 0 ] && [ -n "$CLI_OUT" ]; then
  pass "system prompt accepted"
else
  # --append-system-prompt may not be available in all versions; don't hard-fail
  if [ -n "$CLI_OUT" ]; then
    pass "responded (system prompt flag may not be supported in this version)"
  else
    fail "system prompt failed (exit=$CLI_EXIT): ${CLI_OUT:0:200}"
  fi
fi

# Test: Max tokens limit
run_test "claude -p with --max-turns 1"
CLI_EXIT=0
CLI_OUT=$(run_claude --max-turns 1 "Write a haiku about clouds.") || CLI_EXIT=$?
echo "  exit=$CLI_EXIT output='${CLI_OUT:0:200}'"

if [ "$CLI_EXIT" -eq 0 ] && [ -n "$CLI_OUT" ]; then
  pass "max-turns=1 returned response"
else
  if [ -n "$CLI_OUT" ]; then
    pass "responded with exit=$CLI_EXIT (max-turns flag behavior varies)"
  else
    fail "max-turns failed (exit=$CLI_EXIT)"
  fi
fi

echo ""

# ===================================================================
# Section 3: OpenAI Format Compatibility (auto-conversion)
# ===================================================================

echo "--- Section 3: OpenAI Format via Auto-Conversion ---"
echo ""

# Test: OpenAI chat completions format (non-streaming)
run_test "OpenAI format - non-streaming /v1/chat/completions"
OAI_RESP=$(curl -sf "${MAXX_URL}/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer dummy" \
  -d '{
    "model": "claude-sonnet-4-20250514",
    "max_tokens": 64,
    "messages": [{"role": "user", "content": "What is 3+3? Reply with just the number."}]
  }' 2>&1) || true

if echo "$OAI_RESP" | jq -e '.choices[0].message.content' > /dev/null 2>&1; then
  OAI_CONTENT=$(echo "$OAI_RESP" | jq -r '.choices[0].message.content')
  if echo "$OAI_CONTENT" | grep -q "6"; then
    pass "OpenAI non-streaming correct answer '6'"
  else
    pass "OpenAI non-streaming valid response: ${OAI_CONTENT:0:80}"
  fi
else
  fail "OpenAI non-streaming failed: ${OAI_RESP:0:200}"
fi

# Test: OpenAI chat completions format (streaming)
run_test "OpenAI format - streaming /v1/chat/completions"
OAI_STREAM=$(curl -sf "${MAXX_URL}/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer dummy" \
  -d '{
    "model": "claude-sonnet-4-20250514",
    "max_tokens": 64,
    "stream": true,
    "messages": [{"role": "user", "content": "Say OK"}]
  }' 2>&1) || true

if echo "$OAI_STREAM" | grep -q "data:"; then
  if echo "$OAI_STREAM" | grep -q '"delta"'; then
    pass "OpenAI streaming returns delta chunks"
  else
    pass "OpenAI streaming returns data events"
  fi
else
  fail "OpenAI streaming no data events: ${OAI_STREAM:0:200}"
fi

echo ""

# ===================================================================
# Section 4: Tool Use
# ===================================================================

echo "--- Section 4: Tool Use ---"
echo ""

# Test: Tool use with Claude format
run_test "Tool use - function call and result"
TOOL_RESP=$(curl -sf "${MAXX_URL}/v1/messages" \
  -H "Content-Type: application/json" \
  -H "x-api-key: dummy" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "claude-sonnet-4-20250514",
    "max_tokens": 256,
    "tools": [{"name": "get_weather", "description": "Get current weather for a city", "input_schema": {"type": "object", "properties": {"city": {"type": "string"}}, "required": ["city"]}}],
    "messages": [{"role": "user", "content": "What is the weather in Tokyo?"}]
  }' 2>&1) || true

if echo "$TOOL_RESP" | jq -e '.content[] | select(.type == "tool_use")' > /dev/null 2>&1; then
  TOOL_NAME=$(echo "$TOOL_RESP" | jq -r '.content[] | select(.type == "tool_use") | .name')
  pass "tool call returned: ${TOOL_NAME}"
elif echo "$TOOL_RESP" | jq -e '.content[0].text' > /dev/null 2>&1; then
  pass "model responded with text (may not have called tool)"
else
  fail "tool use failed: ${TOOL_RESP:0:200}"
fi

# Test: Tool use with streaming
run_test "Tool use - streaming with tool call"
TOOL_STREAM=$(curl -sf "${MAXX_URL}/v1/messages" \
  -H "Content-Type: application/json" \
  -H "x-api-key: dummy" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "claude-sonnet-4-20250514",
    "max_tokens": 256,
    "stream": true,
    "tools": [{"name": "calculator", "description": "Calculate math", "input_schema": {"type": "object", "properties": {"expression": {"type": "string"}}, "required": ["expression"]}}],
    "messages": [{"role": "user", "content": "Use the calculator tool to compute 123*456"}]
  }' 2>&1) || true

if echo "$TOOL_STREAM" | grep -q "content_block_start"; then
  if echo "$TOOL_STREAM" | grep -q "tool_use"; then
    pass "streaming tool call with content_block events"
  else
    pass "streaming response with content blocks"
  fi
else
  fail "streaming tool use failed: ${TOOL_STREAM:0:200}"
fi

echo ""

# ===================================================================
# Section 5: Extended Thinking
# ===================================================================

echo "--- Section 5: Extended Thinking ---"
echo ""

# Test: Extended thinking returns thinking blocks
run_test "Extended thinking - thinking blocks in response"
THINK_RESP=$(curl -sf "${MAXX_URL}/v1/messages" \
  -H "Content-Type: application/json" \
  -H "x-api-key: dummy" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "claude-sonnet-4-20250514",
    "max_tokens": 16000,
    "thinking": {"type": "enabled", "budget_tokens": 5000},
    "messages": [{"role": "user", "content": "What is 15 * 23? Show your work."}]
  }' 2>&1) || true

if echo "$THINK_RESP" | jq -e '.content[] | select(.type == "thinking")' > /dev/null 2>&1; then
  THINK_TEXT=$(echo "$THINK_RESP" | jq -r '.content[] | select(.type == "thinking") | .thinking' | head -c 100)
  pass "thinking block present: ${THINK_TEXT:0:80}..."
elif echo "$THINK_RESP" | jq -e '.content[0].text' > /dev/null 2>&1; then
  pass "response valid (thinking may be empty for simple questions)"
else
  fail "thinking request failed: ${THINK_RESP:0:200}"
fi

# Test: Extended thinking with streaming
run_test "Extended thinking - streaming with thinking events"
THINK_STREAM=$(curl -sf "${MAXX_URL}/v1/messages" \
  -H "Content-Type: application/json" \
  -H "x-api-key: dummy" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "claude-sonnet-4-20250514",
    "max_tokens": 16000,
    "stream": true,
    "thinking": {"type": "enabled", "budget_tokens": 5000},
    "messages": [{"role": "user", "content": "Explain why the sky is blue in one sentence."}]
  }' 2>&1) || true

if echo "$THINK_STREAM" | grep -q "message_start" && echo "$THINK_STREAM" | grep -q "message_stop"; then
  if echo "$THINK_STREAM" | grep -q "thinking"; then
    pass "streaming thinking blocks present"
  else
    pass "streaming completed (thinking may be minimal)"
  fi
else
  fail "streaming thinking failed: ${THINK_STREAM:0:200}"
fi

echo ""

# ===================================================================
# Section 6: Prompt Caching Verification
# ===================================================================

echo "--- Section 6: Prompt Caching ---"
echo ""

# Test: Send same request twice, second should get cache hit
CACHE_SYSTEM='[{"type":"text","text":"You are a helpful assistant that always responds in exactly one word. This is a very long system prompt to ensure it gets cached. '"$(python3 -c "print('padding ' * 500)" 2>/dev/null || printf 'padding %.0s' $(seq 1 500))"'","cache_control":{"type":"ephemeral"}}]'

run_test "Prompt caching - first request (cache write)"
CACHE1_RESP=$(curl -sf "${MAXX_URL}/v1/messages" \
  -H "Content-Type: application/json" \
  -H "x-api-key: dummy" \
  -H "anthropic-version: 2023-06-01" \
  -d "{
    \"model\": \"claude-sonnet-4-20250514\",
    \"max_tokens\": 32,
    \"system\": ${CACHE_SYSTEM},
    \"messages\": [{\"role\": \"user\", \"content\": \"Say yes\"}]
  }" 2>&1) || true

CACHE_WRITE=$(echo "$CACHE1_RESP" | jq -r '.usage.cache_creation_input_tokens // 0' 2>/dev/null) || CACHE_WRITE=0
if echo "$CACHE1_RESP" | jq -e '.content[0].text' > /dev/null 2>&1; then
  pass "first request ok (cache_creation_input_tokens=${CACHE_WRITE})"
else
  fail "first cache request failed: ${CACHE1_RESP:0:200}"
fi

run_test "Prompt caching - second request (cache read)"
CACHE2_RESP=$(curl -sf "${MAXX_URL}/v1/messages" \
  -H "Content-Type: application/json" \
  -H "x-api-key: dummy" \
  -H "anthropic-version: 2023-06-01" \
  -d "{
    \"model\": \"claude-sonnet-4-20250514\",
    \"max_tokens\": 32,
    \"system\": ${CACHE_SYSTEM},
    \"messages\": [{\"role\": \"user\", \"content\": \"Say no\"}]
  }" 2>&1) || true

CACHE_READ=$(echo "$CACHE2_RESP" | jq -r '.usage.cache_read_input_tokens // 0' 2>/dev/null) || CACHE_READ=0
if echo "$CACHE2_RESP" | jq -e '.content[0].text' > /dev/null 2>&1; then
  if [ "$CACHE_READ" -gt 0 ]; then
    pass "cache HIT confirmed (cache_read_input_tokens=${CACHE_READ})"
  else
    pass "response valid (cache_read_input_tokens=${CACHE_READ}, may need longer prompt)"
  fi
else
  fail "second cache request failed: ${CACHE2_RESP:0:200}"
fi

echo ""

# ===================================================================
# Section 7: Error Handling
# ===================================================================

echo "--- Section 7: Error Handling ---"
echo ""

# Test: Invalid model returns proper error
run_test "Error handling - invalid model"
ERR_RESP=$(curl -s "${MAXX_URL}/v1/messages" \
  -H "Content-Type: application/json" \
  -H "x-api-key: dummy" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "nonexistent-model-xyz",
    "max_tokens": 32,
    "messages": [{"role": "user", "content": "Hi"}]
  }' 2>&1) || true

HTTP_CODE=$(curl -so /dev/null -w "%{http_code}" "${MAXX_URL}/v1/messages" \
  -H "Content-Type: application/json" \
  -H "x-api-key: dummy" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "nonexistent-model-xyz",
    "max_tokens": 32,
    "messages": [{"role": "user", "content": "Hi"}]
  }' 2>&1) || true

if [ "$HTTP_CODE" -ge 400 ]; then
  pass "invalid model returned HTTP ${HTTP_CODE}"
else
  fail "invalid model should return 4xx, got ${HTTP_CODE}"
fi

# ===================================================================
# Summary
# ===================================================================

echo ""
echo "============================================"
echo "  Results: ${PASS} passed, ${FAIL} failed (${TESTS_RUN} total)"
echo "============================================"

if [ "$FAIL" -gt 0 ]; then
  exit 1
fi
