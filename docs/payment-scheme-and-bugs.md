# NME v9 — 完整支付方案 & 代码分析报告

> 版本：v9 | 生成日期：2026-04-25 | 状态：✅ 已修复部分问题，⚠️ 待修复项已标注

---

## 一、整体架构概览

```
A 站 (商户/店铺)
    │  ① 客户下单 → 携带: 地址、邮箱、IP、金额、OrderID
    ▼
SS 中间站 (NME v9 · Go · Port 5051)
    │  ② 轮询/随机/固定 → 选择 B 站产品 ID
    │  ③ 转发: 产品ID + 邮箱 + IP + 金额 + 支付方式
    ▼
B 站 (WordPress + PayPal/Stripe 插件)
    │  ④ 查询产品，生成订单，返回支付链接
    ▼
SS ← ⑤ 支付链接
    │  ⑥ 支付链接 → 返回给 A 站 → 跳转客户浏览器
    ▼
客户支付 (PayPal / Stripe 收银台)
    │  ⑦ 支付完成
    ▼
B 站 → ⑧ 回调 SS (pay_token + 状态 + 金额)
    │  ⑨ SS 判定支付成功
    │     ├─ 成功 → 3 秒内返回感谢页 HTML / 点击"返回商店"
    │     └─ 超时/放弃 → 3分30秒后标记 abandoned
    ▼
SS → ⑩ 异步回调 A 站订单完成接口
    ▼
A 站 → 订单完成页 (感谢界面)
```

---

## 二、详细下单流程（逐步说明）

### Step 1：A 站发起下单请求 → SS

**接口：** `POST /api/gateway/order`

**请求头（HMAC-SHA256 签名）：**
```
X-Api-Key:    ak_xxx          # A 站 API Key
X-Timestamp:  1714000000      # Unix 秒，允许 ±300 秒误差
X-Signature:  hex(HMAC-SHA256(secret, "$apiKey\n$timestamp\nSHA256($body)"))
```

**请求体：**
```json
{
  "a_order_id":     "SHOP-20240101-001",
  "amount":         99.99,
  "currency":       "USD",
  "payment_method": "paypal",          // stripe | paypal
  "email":          "buyer@email.com",
  "ip":             "1.2.3.4",
  "return_url":     "https://shopA.com/order/success",
  "checkout_url":   "https://shopA.com/checkout"
}
```

**SS 内部处理：**
1. 验证 API Key → 查用户
2. 验证 HMAC 签名（防重放，窗口 300 秒）
3. 幂等检查：同 `user_id + a_order_id` 已存在则直接返回
4. 风控评估：`amount > 5000` → 拦截（⚠️ 见 Bug 列表 #3）
5. 按用户策略选产品（round_robin / random / fixed）
6. 生成 `pay_token`（16 字节随机 hex）
7. 调用 B 站建单接口

---

### Step 2：SS → B 站建单

**接口：** `POST {b_endpoint}/wp-json/b-station/v1/order`

**SS 发出的请求体：**
```json
{
  "a_order_id":     "SHOP-20240101-001",
  "email":          "buyer@email.com",
  "ip":             "1.2.3.4",
  "amount":         "99.99",
  "currency":       "USD",
  "payment_method": "paypal",
  "sandbox":        false
}
```

**B 站返回：**
```json
{
  "payment_url": "https://www.paypal.com/checkoutnow?token=xxx",
  "b_order_id":  "WC-2024-9876",
  "order_id":    "WC-2024-9876"    // 兼容字段
}
```

---

### Step 3：SS 返回支付链接给 A 站

```json
{
  "code": 0,
  "data": {
    "payment_url":    "https://www.paypal.com/checkoutnow?token=xxx",
    "pay_token":      "a3f9e2b1...",
    "order_status":   "pending",
    "payment_method": "paypal",
    "risk_score":     10
  }
}
```

A 站收到后将客户浏览器重定向到 `payment_url`。

---

### Step 4：客户在 PayPal / Stripe 完成支付

- **B 站收到支付成功回调**（来自 PayPal/Stripe Webhook）
- B 站向 SS 回调：

**接口：** `POST /api/gateway/callback`

```json
{
  "pay_token":      "a3f9e2b1...",
  "status":         "completed",
  "amount":         99.99,
  "b_order_id":     "WC-2024-9876",
  "transaction_id": "PAYPAL-TXN-001"
}
```

---

### Step 5：SS 判定支付成功 → 触发感谢页逻辑

**SS 处理：**
1. 用 `pay_token` 查订单，校验状态为 `pending`
2. 比对金额误差（允许 ±0.02）
3. 更新订单为 `completed`，记录 `paid_at`
4. 入队异步任务：`callback:a`（MaxRetry=3）

**感谢页跳转（3秒内）：**
```
B 站在支付成功后，将客户浏览器重定向到：
  https://ss.yourdomain.com/pay/result?token={pay_token}&status=completed

SS /pay/result 页面逻辑：
  ├─ 检查 status=completed → 显示感谢页 HTML
  ├─ 3 秒后自动跳转 return_url（A 站感谢页）
  └─ "返回商店"按钮 → 立即跳转 return_url
```

> ⚠️ **当前问题**：`/pay/result` 仅返回 JSON，未返回完整 HTML 页面（见 Bug #1）

---

### Step 6：放弃支付检测（3分30秒）

**机制：**
- 建单时同步入队延时任务：`check:abandoned`（210秒后执行）
- Worker 执行：若订单仍为 `pending` → 标记 `abandoned`
- 对应 A 站行为：订单超时后重定向回 `checkout_url`（结账界面）

**前端配合逻辑（需在 pay.html 实现）：**
```javascript
// 客户关闭或离开支付页时
// 轮询 /api/gateway/status/{token}，若 status=abandoned → 重定向 checkout_url
```

---

### Step 7：SS 异步回调 A 站

**接口（A 站接收）：** 由 A 站 Webhook 端点配置

**SS 发出：**
```json
{
  "order_id":       "SHOP-20240101-001",
  "b_order_id":     "WC-2024-9876",
  "amount":         "99.99",
  "status":         "paid",
  "nme_order_id":   42,
  "transaction_id": "PAYPAL-TXN-001"
}
```

A 站收到后更新订单状态，展示"感谢界面"。

---

## 三、多渠道负载均衡

### 现有实现

代码中已完整实现三种策略（`internal/service/product_selector.go`）：

| 策略 | 选择逻辑 | 适用场景 |
|------|----------|----------|
| `round_robin`（默认） | 选 `last_used_at` 最旧的产品 | 均匀分流，多账号轮换 |
| `random` | 按 `weight` 权重随机 | 差异化流量分配 |
| `fixed` | 始终选第一个（`poll_order` 最小） | 单一主通道 |

**配置路径：**
- 管理后台 → 用户设置 → 策略：`PUT /api/user/strategy`
- 每个 Product 可设置 `weight`、`poll_order`、`payment_method`（stripe/paypal）
- 每个 Product 可单独配置 B 站账号（支持多 B 站）

**支持 PayPal / Stripe 分流：**
- `WebhookEndpoint.payment_method = "all" | "stripe" | "paypal"`
- 建单时按 `payment_method` 匹配对应 B 站端点

---

## 四、Bug 清单与修复建议

### 🔴 Bug #1（重要）：/pay/result 未返回 HTML 感谢页

**文件：** `internal/handler/pay/handler.go` + `frontend/pay.html`

**问题：**  
`GET /pay/result` 仅返回 JSON `{"token":"...","status":"..."}` 。  
`pay.html` 是前端独立文件（由 nginx 静态服务），它通过 fetch 调用 `/pay/result` 接口拿 JSON 后直接展示原始 JSON，没有实现：
- ✅ 支付成功 → 3 秒后自动跳转 `return_url`
- ✅ 放弃/超时 → 跳回 `checkout_url`
- ✅ "返回商店"按钮

**修复方案：** 重写 `pay.html`，轮询订单状态并实现跳转逻辑（已在下方提供修复版）。

---

### 🔴 Bug #2（安全）：/api/gateway/callback 缺少调用方身份验证

**文件：** `internal/handler/gateway/handler.go` → `Callback()`

**问题：**  
`Callback` 接口使用的是 A 站 API Key 验证（`ResolveUserByAPIKey`），但该接口实际上应由 **B 站** 调用。B 站使用的是 `BApiKey + BSharedSecret`，而 `ResolveUserByAPIKey` 只查 `api_keys` 表（A 站密钥），导致：
- B 站无法正确通过身份验证
- 或者 B 站必须使用 A 站的 Key，造成密钥混用

**修复建议：**  
新增专门的 B 站回调验证逻辑，或为回调接口设置独立的认证路径（如基于 `pay_token` + `BSharedSecret` HMAC 验证）。

---

### 🟡 Bug #3（业务逻辑）：风控规则硬编码，无法动态配置

**文件：** `internal/service/risk.go`

**问题：**
```go
func AssessRisk(amount float64) (int, bool) {
    score := 10
    if amount > 5000 { score = 85 }  // 硬编码！
    return score, score >= 80
}
```
数据库中已有 `risk_rules` 表和完整的 CRUD API，但 `AssessRisk` 完全没有使用它，风控规则形同虚设。

**修复建议：** 将 `DB *gorm.DB` 注入 `AssessRisk`，从 `risk_rules` 表读取规则并评分。

---

### 🟡 Bug #4（并发）：weightedRandom 使用全局 math/rand，无并发安全问题但行为不稳定

**文件：** `internal/service/product_selector.go`

**问题：**  
`rand.Intn(total)` 使用的是 Go 默认的全局随机源（Go 1.20+ 已自动 seed，无安全问题），但在高并发下可能出现多个 goroutine 同时选中同一 product 的情况（round_robin 的 `RecordUsage` 是异步更新，存在短暂不一致窗口）。

**建议：** 对 `round_robin` 策略的更新加分布式锁（Redis SETNX），或改用原子操作（`SELECT FOR UPDATE`）。

---

### 🟡 Bug #5（数据完整性）：建单后 B 站调用失败，订单状态更新为 failed 但不回滚

**文件：** `internal/service/gateway.go` → `CreateOrder()`

**问题：**  
先写入订单记录，再调用 B 站。若 B 站失败：
- 订单变为 `failed` 状态
- `pay_token` 已消耗
- 客户重试时，幂等检查会命中 `failed` 的旧记录并返回 `failed`

**修复建议：** 幂等检查时过滤 `status != 'failed'`，允许对失败订单重试：
```go
s.DB.Where("user_id = ? AND a_order_id = ? AND status != 'failed'", ...).First(&exists)
```

---

### 🟢 Bug #6（轻微）：CallbackAStation 回调状态固定为 "paid"，不区分 failed/abandoned

**文件：** `internal/service/gateway.go` → `CallbackAStation()`

**问题：**
```go
payload := map[string]any{
    "status": "paid",  // 始终为 paid，即使订单是 failed
    ...
}
```
当 `FailByPayToken` 触发 A 站回调时，A 站收到的 `status` 也是 `"paid"`。

**修复：** 传入 `order.Status` 并根据实际状态设置 `status` 字段（`paid` / `failed` / `abandoned`）。

---

### 🟢 Bug #7（轻微）：deploy.sh 使用 prod 配置但无迁移步骤

**文件：** `deploy.sh`

**问题：** 简化版 `deploy.sh` 只执行 `docker compose up`，没有运行 `migrate` 和 `seed`，首次部署会报错（表不存在）。

**修复：** 已合并到新的 `deploy-linux.sh` 中，自动执行迁移。

---

## 五、支付结果页修复方案

修复后的 `pay.html` 实现以下逻辑：

```
轮询 /api/gateway/status/{token}（每 2 秒）
  ├─ status = completed
  │    ├─ 显示"支付成功"感谢页
  │    ├─ 3 秒倒计时后自动跳转 return_url
  │    └─ "立即返回商店"按钮
  ├─ status = abandoned / failed
  │    ├─ 显示"支付未完成"提示
  │    └─ 立即跳转 checkout_url（重新结账）
  └─ status = pending（超过 3分30秒）
       └─ 视为放弃，跳转 checkout_url
```

---

## 六、部署说明

### 端口变更

| 项目 | 旧配置 | 新配置 |
|------|--------|--------|
| APP_PORT（对外） | 8080 | **5051** |
| 容器内端口 | 8080 | 8080（不变） |

### 一键部署命令

```bash
# 开发/测试（HTTP，端口 5051）
sudo bash scripts/deploy-linux.sh

# 生产（HTTPS，自动申请证书）
sudo bash scripts/deploy-linux.sh --domain pay.yourdomain.com --email admin@yourdomain.com
```

### 已删除文件
- ❌ `scripts/install.ps1`（Windows 脚本，已删除）
- ❌ `scripts/deploy-debian12.sh`（已合并）

### 保留文件
- ✅ `scripts/deploy-linux.sh`（统一 Linux 一键部署脚本）

---

## 七、API 签名示例（A 站接入参考）

```python
import hmac, hashlib, time, json, requests

API_KEY = "ak_demo_seed_001"
SECRET  = "sk_demo_seed_001"
SS_URL  = "http://your-ss:5051"

def sign(secret, api_key, ts, body_bytes):
    body_hash = hashlib.sha256(body_bytes).hexdigest()
    payload   = f"{api_key}\n{ts}\n{body_hash}"
    return hmac.new(secret.encode(), payload.encode(), hashlib.sha256).hexdigest()

body = json.dumps({
    "a_order_id":     "ORDER-001",
    "amount":         99.99,
    "currency":       "USD",
    "payment_method": "paypal",
    "email":          "buyer@example.com",
    "ip":             "1.2.3.4",
    "return_url":     "https://shopA.com/thanks",
    "checkout_url":   "https://shopA.com/checkout"
}).encode()

ts  = int(time.time())
sig = sign(SECRET, API_KEY, ts, body)

resp = requests.post(f"{SS_URL}/api/gateway/order",
    data=body,
    headers={
        "Content-Type": "application/json",
        "X-Api-Key":    API_KEY,
        "X-Timestamp":  str(ts),
        "X-Signature":  sig,
    }
)
print(resp.json())
# → {"code":0,"data":{"payment_url":"https://paypal.com/...","pay_token":"...","order_status":"pending"}}
```

---

## 八、总结

| 类别 | 结论 |
|------|------|
| 核心下单流程 | ✅ 完整，A→SS→B→支付→回调→A 链路通 |
| 多渠道负载均衡 | ✅ 已实现（round_robin / random / fixed） |
| PayPal + Stripe 分流 | ✅ 按 payment_method 路由 |
| 放弃支付检测（3分30秒） | ✅ 已实现（asynq 延时队列） |
| 支付成功感谢页 | ⚠️ pay.html 需重写（Bug #1） |
| B 站回调身份验证 | 🔴 需修复（Bug #2） |
| 风控规则 | 🔴 硬编码，需接入 DB（Bug #3） |
| 幂等重试 | ⚠️ 需修复 failed 状态重试（Bug #5） |
| 回调状态值 | ⚠️ 固定 "paid" 需修复（Bug #6） |
| 部署脚本 | ✅ 已合并为单一 Linux 脚本，端口 5051 |
