#!/bin/bash

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 获取最新的 tag
get_latest_tag() {
    git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0"
}

# 解析版本号
parse_version() {
    local version=$1
    version=${version#v}  # 去掉 v 前缀
    IFS='.' read -r major minor patch <<< "$version"
    echo "$major $minor $patch"
}

# bump 版本
bump_version() {
    local major=$1
    local minor=$2
    local patch=$3
    local bump_type=$4

    case $bump_type in
        major)
            major=$((major + 1))
            minor=0
            patch=0
            ;;
        minor)
            minor=$((minor + 1))
            patch=0
            ;;
        patch)
            patch=$((patch + 1))
            ;;
    esac

    echo "v${major}.${minor}.${patch}"
}

# 主流程
main() {
    echo -e "${BLUE}=== Git Tag Bump ===${NC}"
    echo

    # 获取当前版本
    current_tag=$(get_latest_tag)
    echo -e "当前版本: ${GREEN}${current_tag}${NC}"
    echo

    # 解析版本号
    read -r major minor patch <<< "$(parse_version "$current_tag")"

    # 计算各种 bump 后的版本
    patch_version=$(bump_version "$major" "$minor" "$patch" "patch")
    minor_version=$(bump_version "$major" "$minor" "$patch" "minor")
    major_version=$(bump_version "$major" "$minor" "$patch" "major")

    # 交互式选择
    echo "选择版本类型:"
    echo -e "  ${YELLOW}1)${NC} patch  ${GREEN}${patch_version}${NC}"
    echo -e "  ${YELLOW}2)${NC} minor  ${GREEN}${minor_version}${NC}"
    echo -e "  ${YELLOW}3)${NC} major  ${GREEN}${major_version}${NC}"
    echo -e "  ${YELLOW}4)${NC} 取消"
    echo

    read -p "请选择 [1-4]: " choice

    case $choice in
        1)
            new_version=$patch_version
            ;;
        2)
            new_version=$minor_version
            ;;
        3)
            new_version=$major_version
            ;;
        4|*)
            echo -e "${YELLOW}已取消${NC}"
            exit 0
            ;;
    esac

    echo
    echo -e "将创建新 tag: ${GREEN}${new_version}${NC}"
    read -p "确认推送? [y/N]: " confirm

    if [[ ! "$confirm" =~ ^[Yy]$ ]]; then
        echo -e "${YELLOW}已取消${NC}"
        exit 0
    fi

    # 创建并推送 tag
    echo
    echo -e "${BLUE}创建 tag...${NC}"
    git tag "$new_version"

    echo -e "${BLUE}推送 tag...${NC}"
    git push origin "$new_version"

    echo
    echo -e "${GREEN}完成! 新版本: ${new_version}${NC}"
}

main
