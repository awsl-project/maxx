#!/bin/bash

set -e

# 获取参数
if [ -z "$1" ] || [ -z "$2" ]; then
    echo "Usage: $0 <github_token> <version>"
    echo "Example: $0 ghp_xxxx v1.0.0"
    exit 1
fi

GITHUB_TOKEN=$1
VERSION=$2

# 获取仓库信息
REMOTE_URL=$(git config --get remote.origin.url)
REPO=$(echo "$REMOTE_URL" | sed -n 's|.*github.com[:/]\(.*\)\.git|\1|p')

if [ -z "$REPO" ]; then
    echo "Error: 无法解析仓库信息"
    exit 1
fi

echo "Creating tag: $VERSION"
echo "Repository: $REPO"

# 创建并推送 tag
git tag "$VERSION"
git push origin "$VERSION"

echo "Tag $VERSION pushed successfully"

# 创建 GitHub Release
echo "Creating GitHub Release..."

RESPONSE=$(curl -s -X POST \
    -H "Authorization: token $GITHUB_TOKEN" \
    -H "Accept: application/vnd.github.v3+json" \
    "https://api.github.com/repos/$REPO/releases" \
    -d "{
        \"tag_name\": \"$VERSION\",
        \"name\": \"$VERSION\",
        \"generate_release_notes\": true,
        \"draft\": false,
        \"prerelease\": false
    }")

# 检查是否成功
if echo "$RESPONSE" | grep -q '"id"'; then
    RELEASE_URL=$(echo "$RESPONSE" | grep -o '"html_url": "[^"]*"' | head -1 | cut -d'"' -f4)
    echo "Release created successfully: $RELEASE_URL"
else
    echo "Error creating release:"
    echo "$RESPONSE"
    exit 1
fi
