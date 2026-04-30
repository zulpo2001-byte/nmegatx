#!/usr/bin/env bash
set -euo pipefail

# 一键拉取并部署（生产 compose）
# 用法：
#   ./one_click_pull_deploy.sh [branch]
# 示例：
#   ./one_click_pull_deploy.sh release/full-pack-20260430

BRANCH="${1:-release/full-pack-20260430}"

echo "[1/6] Fetch latest from origin..."
git fetch origin

echo "[2/6] Checkout branch: ${BRANCH}"
git checkout "${BRANCH}"

echo "[3/6] Pull latest commits..."
git pull --ff-only origin "${BRANCH}"

echo "[4/6] Build and start containers..."
docker compose -f docker-compose.prod.yml up -d --build

echo "[5/6] Run migrations..."
docker compose -f docker-compose.prod.yml exec -T app ./migrate

echo "[6/6] Seed baseline data (idempotent where applicable)..."
docker compose -f docker-compose.prod.yml exec -T app ./seed || true

echo "Done. Current version: $(cat VERSION 2>/dev/null || echo unknown)"
docker compose -f docker-compose.prod.yml ps
