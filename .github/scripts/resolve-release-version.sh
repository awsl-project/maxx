#!/usr/bin/env bash

set -euo pipefail

trim() {
  local value="$1"
  value="${value#"${value%%[![:space:]]*}"}"
  value="${value%"${value##*[![:space:]]}"}"
  printf '%s' "$value"
}

input_version="$(trim "${1:-}")"
latest_tag="$(git tag --list 'v*' --sort=-version:refname | awk '/^v[0-9]+\.[0-9]+\.[0-9]+$/ { print; exit }')"
semver_pattern='^v([0-9]+)\.([0-9]+)\.([0-9]+)$'

if [[ -z "$input_version" ]]; then
  source_mode="auto"
  if [[ -n "$latest_tag" && "$latest_tag" =~ $semver_pattern ]]; then
    major="${BASH_REMATCH[1]}"
    minor="${BASH_REMATCH[2]}"
    patch="${BASH_REMATCH[3]}"
    resolved_version="v${major}.${minor}.$((patch + 1))"
  else
    resolved_version="v0.0.1"
  fi
else
  source_mode="manual"
  resolved_version="$input_version"
fi

if [[ ! "$resolved_version" =~ $semver_pattern ]]; then
  echo "Error: 版本号格式不正确，应为 vX.Y.Z" >&2
  exit 1
fi

if git rev-parse "$resolved_version" >/dev/null 2>&1; then
  echo "Error: Tag $resolved_version 已存在" >&2
  exit 1
fi

if [[ -n "${GITHUB_OUTPUT:-}" ]]; then
  {
    echo "version=$resolved_version"
    echo "latest_tag=$latest_tag"
    echo "source=$source_mode"
  } >> "$GITHUB_OUTPUT"
fi

echo "$resolved_version"
