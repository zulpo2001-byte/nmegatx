#!/usr/bin/env bash
set -euo pipefail

# 服务器直接写入并运行（域名固定版）
# 目标域名: https://lol.1966908.xyz

REPO_URL="https://github.com/zulpo2001-byte/nmegatx.git"
BRANCH="release/full-pack-20260430"
APP_DIR="/root/nmegatx/nmegatx"
BASE_URL="https://lol.1966908.xyz"

mkdir -p "$(dirname "$APP_DIR")"

if [[ ! -d "$APP_DIR/.git" ]]; then
  echo "[1/7] clone repo..."
  git clone "$REPO_URL" "$APP_DIR"
fi

cd "$APP_DIR"

echo "[2/7] fetch + checkout + pull..."
git fetch origin
git checkout "$BRANCH"
git pull --ff-only origin "$BRANCH"

echo "[3/7] ensure scripts executable..."
chmod +x one_click_pull_deploy.sh verify_deploy.sh online_checklist_admin.sh || true

echo "[4/7] build + up..."
docker compose -f docker-compose.prod.yml up -d --build

echo "[5/7] migrate..."
docker compose -f docker-compose.prod.yml exec -T app ./migrate

echo "[6/7] seed..."
docker compose -f docker-compose.prod.yml exec -T app ./seed || true

echo "[7/7] online checklist on ${BASE_URL} ..."
BASE_URL="$BASE_URL" ADMIN_EMAIL="admin@zulpo.com" ADMIN_PASSWORD="AA123456" ./online_checklist_admin.sh

echo "DONE"
docker compose -f docker-compose.prod.yml ps
