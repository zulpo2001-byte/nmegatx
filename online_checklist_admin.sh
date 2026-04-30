#!/usr/bin/env bash
set -euo pipefail

# 真实线上检查清单（admin 默认账号版，不修改密码）
# 用法：
#   BASE_URL=https://your-domain.com ./online_checklist_admin.sh

BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"
ADMIN_EMAIL="${ADMIN_EMAIL:-admin@zulpo.com}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-AA123456}"

echo "== [0] 基础信息 =="
echo "BASE_URL=${BASE_URL}"
echo "ADMIN_EMAIL=${ADMIN_EMAIL}"

echo "== [1] 健康检查 =="
curl -fsS "${BASE_URL}/api/health" | tee /tmp/check_health.json >/dev/null
echo "OK: /api/health"

echo "== [2] admin 登录 =="
LOGIN_JSON=$(curl -fsS -X POST "${BASE_URL}/api/auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"${ADMIN_EMAIL}\",\"password\":\"${ADMIN_PASSWORD}\"}")
ADMIN_TOKEN=$(echo "$LOGIN_JSON" | sed -n 's/.*"access_token":"\([^"]*\)".*/\1/p')
if [[ -z "${ADMIN_TOKEN}" ]]; then
  echo "FAIL: admin 登录失败，返回：$LOGIN_JSON"
  exit 1
fi
echo "OK: admin 登录成功"

auth_get () {
  local path="$1"
  curl -fsS "${BASE_URL}${path}" -H "Authorization: Bearer ${ADMIN_TOKEN}"
}

auth_post () {
  local path="$1"
  local payload="${2:-{}}"
  curl -fsS -X POST "${BASE_URL}${path}" -H "Authorization: Bearer ${ADMIN_TOKEN}" -H 'Content-Type: application/json' -d "$payload"
}

echo "== [3] 后台关键接口 =="
auth_get "/api/admin/stats" | tee /tmp/check_admin_stats.json >/dev/null
echo "OK: /api/admin/stats"

auth_get "/api/admin/orders" | tee /tmp/check_admin_orders.json >/dev/null
echo "OK: /api/admin/orders"

auth_get "/api/admin/users" | tee /tmp/check_admin_users.json >/dev/null
echo "OK: /api/admin/users"

auth_get "/api/admin/settings" | tee /tmp/check_admin_settings.json >/dev/null
echo "OK: /api/admin/settings"

echo "== [4] 运营/风控接口 =="
auth_get "/api/admin/risk-rules" | tee /tmp/check_risk_rules.json >/dev/null
echo "OK: /api/admin/risk-rules"

auth_get "/api/admin/alerts" | tee /tmp/check_alerts.json >/dev/null
echo "OK: /api/admin/alerts"

auth_get "/api/admin/reports/daily" | tee /tmp/check_reports_daily.json >/dev/null
echo "OK: /api/admin/reports/daily"

echo "== [5] 网关运维接口（只检查可达，不做破坏） =="
auth_get "/api/admin/orders/reset-mode" | tee /tmp/check_reset_mode.json >/dev/null
echo "OK: /api/admin/orders/reset-mode"

echo "== [6] 指标接口（无 Redis 时允许失败） =="
set +e
METRICS_SUMMARY=$(auth_get "/api/admin/metrics/summary" 2>&1)
METRICS_CODE=$?
set -e
if [[ $METRICS_CODE -eq 0 ]]; then
  echo "OK: /api/admin/metrics/summary"
else
  echo "WARN: /api/admin/metrics/summary 不可用（常见原因：redis 未连接）"
fi

echo "== [7] 鉴权保护检查（应返回 401） =="
set +e
NOAUTH_CODE=$(curl -s -o /tmp/check_noauth.out -w "%{http_code}" "${BASE_URL}/api/admin/stats")
set -e
if [[ "$NOAUTH_CODE" == "401" ]]; then
  echo "OK: 未授权访问被正确拦截 (401)"
else
  echo "WARN: 未授权访问返回 ${NOAUTH_CODE}，请检查网关/鉴权中间件"
fi

echo "== [8] 完成 =="
echo "线上检查完成。关键结果文件在 /tmp/check_*.json"
echo "建议人工复核："
echo "  - /tmp/check_admin_stats.json"
echo "  - /tmp/check_admin_orders.json"
echo "  - /tmp/check_admin_users.json"
echo "  - /tmp/check_reports_daily.json"
