#!/usr/bin/env bash
set -euo pipefail

# 全链路网关验证：下单 -> 回调 -> 状态查询
# 依赖: curl, openssl, sed
# 用法示例：
# BASE_URL=http://127.0.0.1:8080 \
# A_API_KEY=ak_demo_user_001 A_API_SECRET=sk_demo_user_001 \
# B_API_KEY=bk_xxx B_API_SECRET=bsk_xxx \
# ./gateway_e2e_verify.sh

BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"
A_API_KEY="${A_API_KEY:-}"
A_API_SECRET="${A_API_SECRET:-}"
B_API_KEY="${B_API_KEY:-}"
B_API_SECRET="${B_API_SECRET:-}"

if [[ -z "$A_API_KEY" || -z "$A_API_SECRET" || -z "$B_API_KEY" || -z "$B_API_SECRET" ]]; then
  echo "ERROR: 请设置 A_API_KEY/A_API_SECRET/B_API_KEY/B_API_SECRET"
  exit 1
fi

hmac_hex() {
  local secret="$1"
  local msg="$2"
  printf '%s' "$msg" | openssl dgst -sha256 -hmac "$secret" -binary | xxd -p -c 256
}

sign_body_scheme() {
  local api_key="$1"
  local secret="$2"
  local ts="$3"
  local body="$4"
  local body_hash payload
  body_hash=$(printf '%s' "$body" | openssl dgst -sha256 -binary | xxd -p -c 256)
  payload="${api_key}\n${ts}\n${body_hash}"
  hmac_hex "$secret" "$payload"
}

extract_json_field() {
  local key="$1"
  sed -n "s/.*\"${key}\":\"\([^\"]*\)\".*/\1/p"
}

echo "[1/4] 下单 /api/gateway/order"
TS1=$(date +%s)
A_ORDER_ID="A-E2E-$(date +%s)"
ORDER_BODY=$(cat <<JSON
{"a_order_id":"${A_ORDER_ID}","amount":12.34,"return_url":"https://a.local/thanks","checkout_url":"https://a.local/checkout"}
JSON
)
SIG1=$(sign_body_scheme "$A_API_KEY" "$A_API_SECRET" "$TS1" "$ORDER_BODY")

ORDER_RESP=$(curl -fsS -X POST "${BASE_URL}/api/gateway/order" \
  -H "Content-Type: application/json" \
  -H "X-Api-Key: ${A_API_KEY}" \
  -H "X-Timestamp: ${TS1}" \
  -H "X-Signature: ${SIG1}" \
  -d "$ORDER_BODY")

echo "$ORDER_RESP"
PAY_TOKEN=$(echo "$ORDER_RESP" | extract_json_field "pay_token")
if [[ -z "$PAY_TOKEN" ]]; then
  echo "ERROR: 下单未返回 pay_token"
  exit 1
fi
echo "OK: pay_token=${PAY_TOKEN}"

echo "[2/4] 回调 /api/gateway/callback (completed)"
TS2=$(date +%s)
CALLBACK_BODY=$(cat <<JSON
{"pay_token":"${PAY_TOKEN}","status":"completed","amount":12.34,"b_order_id":"B-E2E-$(date +%s)","transaction_id":"TX-E2E-$(date +%s)"}
JSON
)
SIG2=$(sign_body_scheme "$B_API_KEY" "$B_API_SECRET" "$TS2" "$CALLBACK_BODY")

CALLBACK_RESP=$(curl -fsS -X POST "${BASE_URL}/api/gateway/callback" \
  -H "Content-Type: application/json" \
  -H "X-Api-Key: ${B_API_KEY}" \
  -H "X-Timestamp: ${TS2}" \
  -H "X-Signature: ${SIG2}" \
  -d "$CALLBACK_BODY")

echo "$CALLBACK_RESP"

echo "[3/4] 查询状态 /api/gateway/status/:token"
STATUS_RESP=$(curl -fsS "${BASE_URL}/api/gateway/status/${PAY_TOKEN}")
echo "$STATUS_RESP"
STATUS=$(echo "$STATUS_RESP" | extract_json_field "status")
if [[ "$STATUS" != "completed" ]]; then
  echo "ERROR: 期望 completed，实际=${STATUS:-<empty>}"
  exit 1
fi

echo "[4/4] 全链路验证通过 ✅"
