#!/usr/bin/env bash
set -euo pipefail

INSTALL_DIR="${INSTALL_DIR:-/opt/cc-panel}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

echo "==> CC Panel 安装到 ${INSTALL_DIR}"

if [[ $EUID -ne 0 ]]; then
  echo "请使用 root 运行: sudo bash install.sh"
  exit 1
fi

mkdir -p "${INSTALL_DIR}"
cp -a "${SCRIPT_DIR}/bin" "${INSTALL_DIR}/"
cp -a "${SCRIPT_DIR}/migrations" "${INSTALL_DIR}/"
cp -a "${SCRIPT_DIR}/web" "${INSTALL_DIR}/"
cp -a "${SCRIPT_DIR}/data" "${INSTALL_DIR}/"

if [[ ! -f "${INSTALL_DIR}/.env" ]]; then
  cp "${SCRIPT_DIR}/.env.example" "${INSTALL_DIR}/.env"
  sed -i "s|IP2REGION_V4_XDB=.*|IP2REGION_V4_XDB=${INSTALL_DIR}/data/ipdb/ip2region_v4.xdb|" "${INSTALL_DIR}/.env"
  sed -i "s|IP2REGION_V6_XDB=.*|IP2REGION_V6_XDB=${INSTALL_DIR}/data/ipdb/ip2region_v6.xdb|" "${INSTALL_DIR}/.env"
  echo ""
  echo "!!! 请编辑 ${INSTALL_DIR}/.env 设置："
  echo "    - DATABASE_URL（PostgreSQL 连接）"
  echo "    - JWT_SECRET（至少 32 字符）"
  echo "    - APP_ENCRYPTION_KEY（正好 32 字符）"
  echo "    - ADMIN_PASSWORD（管理员密码）"
  echo ""
fi

echo "==> 检查 PostgreSQL"
if ! command -v psql &>/dev/null; then
  echo "未检测到 psql，请先安装 PostgreSQL 13+ 并创建数据库，例如："
  echo ""
  echo "  sudo -u postgres psql <<'SQL'"
  echo "  CREATE USER cc_panel WITH PASSWORD 'your_password';"
  echo "  CREATE DATABASE cc_panel OWNER cc_panel;"
  echo "  SQL"
  echo ""
else
  echo "    psql 已安装"
fi

echo "==> 运行数据库迁移"
cd "${INSTALL_DIR}"
set -a
source .env
set +a
./bin/cc-panel-migrate

echo "==> 安装 systemd 服务"
if [[ -f "${SCRIPT_DIR}/cc-panel.service" ]]; then
  sed "s|/opt/cc-panel|${INSTALL_DIR}|g" "${SCRIPT_DIR}/cc-panel.service" > /etc/systemd/system/cc-panel.service
  systemctl daemon-reload
  systemctl enable cc-panel
  echo "    服务已注册，启动: systemctl start cc-panel"
else
  echo "    无 systemd 文件，手动启动:"
  echo "    cd ${INSTALL_DIR} && ./bin/cc-panel-server"
fi

echo ""
echo "安装完成！"
echo "  目录: ${INSTALL_DIR}"
echo "  配置: ${INSTALL_DIR}/.env"
echo "  启动: systemctl start cc-panel  或  cd ${INSTALL_DIR} && ./bin/cc-panel-server"
echo "  访问: http://服务器IP:8080"
