#!/usr/bin/env bash
# Setup maxx with Bedrock provider and routes via admin API
set -euo pipefail

MAXX_URL="${MAXX_URL:?MAXX_URL is required}"
MAXX_ADMIN_PASSWORD="${MAXX_ADMIN_PASSWORD:?MAXX_ADMIN_PASSWORD is required}"
AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID:?AWS_ACCESS_KEY_ID is required}"
AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY:?AWS_SECRET_ACCESS_KEY is required}"
AWS_REGION="${AWS_REGION:-us-east-1}"

echo "=== Setting up maxx at ${MAXX_URL} ==="

# 1. Login to get JWT token
echo "--- Logging in ---"
LOGIN_RESP=$(curl -sf "${MAXX_URL}/api/admin/auth/login" \
  -H "Content-Type: application/json" \
  -d "{\"username\":\"admin\",\"password\":\"${MAXX_ADMIN_PASSWORD}\"}")

TOKEN=$(echo "$LOGIN_RESP" | jq -r '.token // empty')
if [ -z "$TOKEN" ]; then
  echo "FAIL: Login failed"
  echo "$LOGIN_RESP"
  exit 1
fi
echo "OK: Got admin token"

# 2. Create Bedrock provider
echo "--- Creating Bedrock provider ---"
PROVIDER_RESP=$(curl -sf "${MAXX_URL}/api/admin/providers" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${TOKEN}" \
  -d "{
    \"name\": \"bedrock-test\",
    \"type\": \"bedrock\",
    \"config\": {
      \"bedrock\": {
        \"accessKeyId\": \"${AWS_ACCESS_KEY_ID}\",
        \"secretAccessKey\": \"${AWS_SECRET_ACCESS_KEY}\",
        \"region\": \"${AWS_REGION}\"
      }
    },
    \"supportedClientTypes\": [\"claude\"]
  }")

PROVIDER_ID=$(echo "$PROVIDER_RESP" | jq -r '.id // empty')
if [ -z "$PROVIDER_ID" ]; then
  echo "FAIL: Create provider failed"
  echo "$PROVIDER_RESP"
  exit 1
fi
echo "OK: Created provider ID=${PROVIDER_ID}"

# 3. Create routes
echo "--- Creating routes ---"

# Claude native route
ROUTE_RESP=$(curl -sf "${MAXX_URL}/api/admin/routes" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${TOKEN}" \
  -d "{
    \"isEnabled\": true,
    \"isNative\": true,
    \"clientType\": \"claude\",
    \"providerID\": ${PROVIDER_ID},
    \"position\": 1,
    \"weight\": 1
  }")
echo "OK: Claude route ID=$(echo "$ROUTE_RESP" | jq -r '.id // "?"')"

# OpenAI route (with format conversion)
ROUTE_RESP=$(curl -sf "${MAXX_URL}/api/admin/routes" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${TOKEN}" \
  -d "{
    \"isEnabled\": true,
    \"isNative\": false,
    \"clientType\": \"openai\",
    \"providerID\": ${PROVIDER_ID},
    \"position\": 1,
    \"weight\": 1
  }")
echo "OK: OpenAI route ID=$(echo "$ROUTE_RESP" | jq -r '.id // "?"')"

# 4. Export token for use by test scripts
echo "$TOKEN" > /tmp/maxx_token
echo "=== Setup complete ==="
