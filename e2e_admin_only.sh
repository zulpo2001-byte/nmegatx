#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"
ADMIN_EMAIL="${ADMIN_EMAIL:-admin@zulpo.com}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-AA123456}"

A_API_KEY="${A_API_KEY:-}"
A_API_SECRET="${A_API_SECRET:-}"
B_API_KEY="${B_API_KEY:-}"
B_API_SECRET="${B_API_SECRET:-}"

FIXED_IP="${FIXED_IP:-1.11.1.1}"
AMOUNT="${AMOUNT:-30.00}"

AUTO_MOCK="${AUTO_MOCK:-0}"
A_MOCK_PORT="${A_MOCK_PORT:-19081}"
B_MOCK_PORT="${B_MOCK_PORT:-19082}"
DOCKER_HOST_ALIAS="${DOCKER_HOST_ALIAS:-host.docker.internal}"

A_MOCK_URL_HOST="http://127.0.0.1:${A_MOCK_PORT}/callback"
B_MOCK_BASE_HOST="http://127.0.0.1:${B_MOCK_PORT}"
A_MOCK_URL_DOCKER="http://${DOCKER_HOST_ALIAS}:${A_MOCK_PORT}/callback"
B_MOCK_BASE_DOCKER="http://${DOCKER_HOST_ALIAS}:${B_MOCK_PORT}"

hmac_hex() {
  local secret="$1" msg="$2"
  printf '%s' "$msg" | openssl dgst -sha256 -hmac "$secret" | awk '{print $2}'
}

sha256_hex() {
  local body="$1"
  printf '%s' "$body" | openssl dgst -sha256 | awk '{print $2}'
}

sign_body_scheme() {
  local api_key="$1" secret="$2" ts="$3" body="$4"
  local body_hash payload
  body_hash=$(sha256_hex "$body")
  payload="${api_key}"$'\n'"${ts}"$'\n'"${body_hash}"
  hmac_hex "$secret" "$payload"
}

extract_json_field() {
  local key="$1"
  sed -n "s/.*\"${key}\":\"\\([^\"]*\\)\".*/\\1/p"
}

echo "[1/10] admin 登录..."
ADMIN_LOGIN_JSON=$(curl -fsS -X POST "${BASE_URL}/api/auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"${ADMIN_EMAIL}\",\"password\":\"${ADMIN_PASSWORD}\"}")
ADMIN_TOKEN=$(echo "$ADMIN_LOGIN_JSON" | extract_json_field "access_token")
[[ -n "$ADMIN_TOKEN" ]] || { echo "ERROR: admin 登录失败: $ADMIN_LOGIN_JSON"; exit 1; }

auth_post() {
  local path="$1" body="$2"
  curl -fsS -X POST "${BASE_URL}${path}" \
    -H "Authorization: Bearer ${ADMIN_TOKEN}" \
    -H 'Content-Type: application/json' \
    -d "$body"
}

if [[ "$AUTO_MOCK" == "1" ]]; then
  echo "[2/10] 启动 A/B mock server..."
  cat >/tmp/mock_ab_server.py <<'PY'
import json
from http.server import BaseHTTPRequestHandler, HTTPServer
import threading, time

A_HITS = []
B_ORDER_HITS = []
B_COMPLETE_HITS = []

class AHandler(BaseHTTPRequestHandler):
    def do_POST(self):
        l = int(self.headers.get('Content-Length', '0'))
        b = self.rfile.read(l).decode('utf-8', 'ignore')
        A_HITS.append({"path": self.path, "body": b})
        self.send_response(200)
        self.send_header('Content-Type', 'application/json')
        self.end_headers()
        self.wfile.write(b'{"ok":true}')

class BHandler(BaseHTTPRequestHandler):
    def do_POST(self):
        l = int(self.headers.get('Content-Length', '0'))
        b = self.rfile.read(l).decode('utf-8', 'ignore')
        if self.path == "/wp-json/b-station/v1/order":
            B_ORDER_HITS.append({"body": b})
            d = json.dumps({"payment_url": "https://b.mock/pay/1", "b_order_id": "B-MOCK-1"}).encode()
            self.send_response(200)
            self.send_header('Content-Type', 'application/json')
            self.send_header('Content-Length', str(len(d)))
            self.end_headers()
            self.wfile.write(d)
            return
        if self.path == "/wp-json/b-station/v1/complete":
            B_COMPLETE_HITS.append({"body": b})
            self.send_response(200)
            self.send_header('Content-Type', 'application/json')
            self.end_headers()
            self.wfile.write(b'{"ok":true}')
            return
        self.send_response(404)
        self.end_headers()

def serve(p, h):
    HTTPServer(("0.0.0.0", p), h).serve_forever()

if __name__ == "__main__":
    import sys
    a = int(sys.argv[1])
    b = int(sys.argv[2])
    threading.Thread(target=serve, args=(a, AHandler), daemon=True).start()
    threading.Thread(target=serve, args=(b, BHandler), daemon=True).start()
    while True:
        print(json.dumps({
            "a_hits": len(A_HITS),
            "b_order_hits": len(B_ORDER_HITS),
            "b_complete_hits": len(B_COMPLETE_HITS)
        }), flush=True)
        time.sleep(2)
PY

  python3 /tmp/mock_ab_server.py "$A_MOCK_PORT" "$B_MOCK_PORT" >/tmp/mock_ab_stat.log 2>&1 &
  MOCK_PID=$!
  trap 'kill ${MOCK_PID} >/dev/null 2>&1 || true' EXIT
  sleep 1

  echo "[3/10] 本机预检查 mock 可达..."
  curl -sS -o /dev/null -w "A mock http=%{http_code}\n" -X POST "${A_MOCK_URL_HOST}" -d '{}' || true
  curl -sS -o /dev/null -w "B order mock http=%{http_code}\n" -X POST "${B_MOCK_BASE_HOST}/wp-json/b-station/v1/order" -d '{}' || true
else
  echo "[2/10] 跳过 mock（AUTO_MOCK=0）"
fi

echo "[4/10] 自动获取/创建 A_API_KEY + A_API_SECRET..."
if [[ -z "$A_API_KEY" || -z "$A_API_SECRET" ]]; then
  NEW_KEY_JSON=$(auth_post "/api/user/api-keys" "{}")
  A_API_KEY=$(echo "$NEW_KEY_JSON" | extract_json_field "api_key")
  A_API_SECRET=$(echo "$NEW_KEY_JSON" | extract_json_field "secret")
fi
[[ -n "$A_API_KEY" && -n "$A_API_SECRET" ]] || { echo "ERROR: A_API_KEY/A_API_SECRET 为空"; exit 1; }
echo "A_API_KEY=${A_API_KEY}"

echo "[5/10] 自动获取/创建 B_API_KEY + B_API_SECRET..."
if [[ -z "$B_API_KEY" || -z "$B_API_SECRET" ]]; then
  if [[ "$AUTO_MOCK" == "1" ]]; then
    B_URL="${B_MOCK_BASE_DOCKER}"
  else
    B_URL="${B_WEBHOOK_URL:-}"
    [[ -n "$B_URL" ]] || { echo "ERROR: 非 AUTO_MOCK 模式请提供 B_WEBHOOK_URL"; exit 1; }
  fi
  B_WEBHOOK_JSON=$(auth_post "/api/user/webhooks/b" "{\"label\":\"B-E2E-AUTO\",\"url\":\"${B_URL}\",\"payment_method\":\"all\"}")
  B_API_KEY=$(echo "$B_WEBHOOK_JSON" | extract_json_field "b_api_key")
  B_API_SECRET=$(echo "$B_WEBHOOK_JSON" | extract_json_field "b_shared_secret")
fi
[[ -n "$B_API_KEY" && -n "$B_API_SECRET" ]] || { echo "ERROR: B_API_KEY/B_API_SECRET 为空"; exit 1; }
echo "B_API_KEY=${B_API_KEY}"

if [[ "$AUTO_MOCK" == "1" ]]; then
  echo "[6/10] AUTO_MOCK 模式：创建 A webhook 指向容器可访问地址..."
  auth_post "/api/user/webhooks/a" "{\"label\":\"A-E2E-AUTO\",\"url\":\"${A_MOCK_URL_DOCKER}\",\"payment_method\":\"all\"}" >/tmp/a_webhook_auto.json || true
fi

run_flow() {
  local pay_method="$1"
  echo "========== FLOW: ${pay_method} =========="

  local ts1 order_body sig1 order_resp pay_token ts2 callback_body sig2 callback_resp status_resp status

  ts1=$(date +%s)
  order_body=$(cat <<JSON
{"a_order_id":"A-ADMIN-E2E-${pay_method}-$(date +%s)","amount":${AMOUNT},"payment_method":"${pay_method}","ip":"${FIXED_IP}","return_url":"https://a.local/thanks","checkout_url":"https://a.local/checkout"}
JSON
)
  sig1=$(sign_body_scheme "$A_API_KEY" "$A_API_SECRET" "$ts1" "$order_body")

  order_resp=$(curl -sS -X POST "${BASE_URL}/api/gateway/order" \
    -H "Content-Type: application/json" \
    -H "X-Api-Key: ${A_API_KEY}" \
    -H "X-Timestamp: ${ts1}" \
    -H "X-Signature: ${sig1}" \
    -d "$order_body")
  echo "order_resp=${order_resp}"

  if echo "$order_resp" | grep -q '"ok":false'; then
    echo "ERROR: 下单失败"
    exit 1
  fi

  pay_token=$(echo "$order_resp" | extract_json_field "pay_token")
  [[ -n "$pay_token" ]] || { echo "ERROR: 无 pay_token"; exit 1; }

  ts2=$(date +%s)
  callback_body=$(cat <<JSON
{"pay_token":"${pay_token}","status":"completed","amount":${AMOUNT},"b_order_id":"B-ADMIN-E2E-${pay_method}-$(date +%s)","transaction_id":"TX-ADMIN-E2E-${pay_method}-$(date +%s)"}
JSON
)
  sig2=$(sign_body_scheme "$B_API_KEY" "$B_API_SECRET" "$ts2" "$callback_body")

  callback_resp=$(curl -sS -X POST "${BASE_URL}/api/gateway/callback" \
    -H "Content-Type: application/json" \
    -H "X-Api-Key: ${B_API_KEY}" \
    -H "X-Timestamp: ${ts2}" \
    -H "X-Signature: ${sig2}" \
    -d "$callback_body")
  echo "callback_resp=${callback_resp}"

  status_resp=$(curl -sS "${BASE_URL}/api/gateway/status/${pay_token}")
  echo "status_resp=${status_resp}"
  status=$(echo "$status_resp" | extract_json_field "status")
  [[ "$status" == "completed" ]] || { echo "ERROR: 状态不是 completed"; exit 1; }

  echo "FLOW ${pay_method} OK"
}

echo "[7/10] Stripe 流程..."
run_flow "stripe"

echo "[8/10] PayPal 流程..."
run_flow "paypal"

echo "[9/10] 全链路验证通过 ✅"

if [[ "$AUTO_MOCK" == "1" ]]; then
  echo "[10/10] mock 命中统计:"
  tail -n 5 /tmp/mock_ab_stat.log || true
fi
