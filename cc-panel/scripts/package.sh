#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

GO="${GO:-/usr/local/go/bin/go}"
if ! command -v "$GO" &>/dev/null; then
  GO=go
fi

VERSION="$(date +%Y%m%d-%H%M%S)"
OUT_DIR="/tmp/cc-panel-release-${VERSION}"
PKG_NAME="cc-panel-linux-amd64-${VERSION}.tar.gz"
PKG_PATH="/tmp/${PKG_NAME}"

echo "==> 构建前端"
if [[ -x web/node_modules/.bin/vite ]]; then
  (cd web && ./node_modules/.bin/vite build)
elif [[ -d web/dist/index.html ]]; then
  echo "    跳过前端构建（使用现有 web/dist）"
else
  echo "错误: 无 web/dist，请先构建前端" >&2
  exit 1
fi

echo "==> 编译 Go 二进制"
mkdir -p "${OUT_DIR}/bin"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 "$GO" build -ldflags="-s -w" -o "${OUT_DIR}/bin/cc-panel-server" ./cmd/server
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 "$GO" build -ldflags="-s -w" -o "${OUT_DIR}/bin/cc-panel-migrate" ./cmd/migrate

echo "==> 复制 SQL 迁移"
cp -a migrations "${OUT_DIR}/"

echo "==> 复制前端静态文件"
mkdir -p "${OUT_DIR}/web"
cp -a web/dist "${OUT_DIR}/web/"

echo "==> 复制 ip2region 数据库"
mkdir -p "${OUT_DIR}/data/ipdb"
cp -a data/ipdb/*.xdb "${OUT_DIR}/data/ipdb/"

echo "==> 复制配置模板与安装脚本"
cp .env.example "${OUT_DIR}/"
cp scripts/install.sh "${OUT_DIR}/"
cp scripts/cc-panel.service "${OUT_DIR}/" 2>/dev/null || true

echo "==> 打包"
tar -czf "${PKG_PATH}" -C "$(dirname "$OUT_DIR")" "$(basename "$OUT_DIR")"

echo ""
echo "打包完成: ${PKG_PATH}"
ls -lh "${PKG_PATH}"
echo ""
echo "部署步骤:"
echo "  1. scp ${PKG_PATH} root@新服务器:/opt/"
echo "  2. ssh 新服务器"
echo "  3. cd /opt && tar xzf ${PKG_NAME}"
echo "  4. cd cc-panel-release-${VERSION} && bash install.sh"
