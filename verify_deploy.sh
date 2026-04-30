#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"
ADMIN_EMAIL="${ADMIN_EMAIL:-admin@zulpo.com}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-AA123456}"
USER_EMAIL="${USER_EMAIL:-user@zulpo.com}"
USER_PASSWORD="${USER_PASSWORD:-AA123456}"

echo "[1/8] health check..."
curl -fsS "${BASE_URL}/api/health" >/tmp/health.json
cat /tmp/health.json

echo "[2/8] admin login..."
ADMIN_LOGIN_JSON=$(curl -fsS -X POST "${BASE_URL}/api/auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"${ADMIN_EMAIL}\",\"password\":\"${ADMIN_PASSWORD}\"}")
ADMIN_TOKEN=$(echo "$ADMIN_LOGIN_JSON" | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p')
[ -n "$ADMIN_TOKEN" ] || { echo "admin token empty"; exit 1; }

echo "[3/8] user login..."
USER_LOGIN_JSON=$(curl -fsS -X POST "${BASE_URL}/api/auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"${USER_EMAIL}\",\"password\":\"${USER_PASSWORD}\"}")
USER_TOKEN=$(echo "$USER_LOGIN_JSON" | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p')
[ -n "$USER_TOKEN" ] || { echo "user token empty"; exit 1; }

echo "[4/8] admin stats..."
curl -fsS "${BASE_URL}/api/admin/stats" -H "Authorization: Bearer ${ADMIN_TOKEN}" >/tmp/admin_stats.json
cat /tmp/admin_stats.json

echo "[5/8] user dashboard..."
curl -fsS "${BASE_URL}/api/user/dashboard" -H "Authorization: Bearer ${USER_TOKEN}" >/tmp/user_dash.json
cat /tmp/user_dash.json

echo "[6/8] user paypal list..."
curl -fsS "${BASE_URL}/api/user/paypal" -H "Authorization: Bearer ${USER_TOKEN}" >/tmp/user_paypal.json
cat /tmp/user_paypal.json

echo "[7/8] user stripe list..."
curl -fsS "${BASE_URL}/api/user/stripe" -H "Authorization: Bearer ${USER_TOKEN}" >/tmp/user_stripe.json
cat /tmp/user_stripe.json

echo "[8/8] user api-keys list..."
curl -fsS "${BASE_URL}/api/user/api-keys" -H "Authorization: Bearer ${USER_TOKEN}" >/tmp/user_keys.json
cat /tmp/user_keys.json

echo "All checks passed on ${BASE_URL}"
