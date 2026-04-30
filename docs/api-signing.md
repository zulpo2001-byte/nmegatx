# Gateway API Signing

本文档定义 `NME v9` 网关接口签名规则。

## Headers

所有网关请求都需要以下请求头：

- `X-Api-Key`: 商户 API Key
- `X-Timestamp`: Unix 秒级时间戳
- `X-Signature`: HMAC-SHA256 十六进制签名

## Time Window

- 默认允许时间偏差 `±300` 秒（`HMAC_WINDOW_SECONDS`）。
- 超出时间窗会被拒绝。

## Order API

- 路由：`POST /api/gateway/order`
- 签名原文：
  - `a_order_id|timestamp|api_key`
- 算法：
  - `signature = hex(hmac_sha256(api_secret, raw_string))`

示例（伪代码）：

```text
raw = "A20260424-1001|1713950000|ak_xxx"
sig = HMAC_SHA256_HEX(api_secret, raw)
```

## Callback API

- 路由：`POST /api/gateway/callback`
- 签名原文：
  - `pay_token|status|timestamp|api_key`
- 算法：
  - `signature = hex(hmac_sha256(api_secret, raw_string))`

示例（伪代码）：

```text
raw = "9c3f...|completed|1713950200|ak_xxx"
sig = HMAC_SHA256_HEX(api_secret, raw)
```

## Callback Status Constraints

`/api/gateway/callback` 仅接受以下状态：

- `completed`
- `failed`
- `abandoned`

其余状态会返回 `400 unsupported status`。
