#!/usr/bin/env bash
# Run integration tests with Docker Compose
# Usage: ./tests/integration/run.sh
#
# Requires .env file in project root with:
#   AWS_ACCESS_KEY_ID=...
#   AWS_SECRET_ACCESS_KEY=...
#   AWS_REGION=us-east-1  (optional)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Load .env from project root
if [ -f "$PROJECT_DIR/.env" ]; then
  set -a
  source "$PROJECT_DIR/.env"
  set +a
fi

if [ -z "${AWS_ACCESS_KEY_ID:-}" ] || [ -z "${AWS_SECRET_ACCESS_KEY:-}" ]; then
  echo "Error: AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY must be set in .env or environment"
  exit 1
fi

echo "Building and running integration tests..."
echo ""

cd "$SCRIPT_DIR"

# Build and run
docker compose up --build --abort-on-container-exit --exit-code-from test-runner

# Cleanup
docker compose down -v
