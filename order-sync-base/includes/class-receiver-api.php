<?php
defined('ABSPATH') || exit;

/**
 * OSB_Receiver_API v7.1
 *
 * 鉴权变更（v7.1）：
 *   旧：X-NME-Api-Key（明文）+ X-NME-Api-Secret（sha256哈希）双头 + complete的 body.sign 二次验签
 *   新：统一 HMAC Header 验签，三个方向公式完全一致
 *       X-Api-Key / X-Timestamp / X-Signature
 *       payload   = api_key + "\n" + timestamp + "\n" + sha256(body)
 *       signature = HMAC-SHA256(payload, b_shared_secret)
 *       ±300s 时间戳防重放窗口
 *
 * 配置字段简化：
 *   旧：b_api_key + b_api_secret（存哈希）+ shared_secret（3个密钥）
 *   新：b_api_key（明文标识符）+ b_shared_secret（唯一签名密钥）
 *
 * 端点：
 *   POST /wp-json/b-station/v1/order    — NME建单通知
 *   POST /wp-json/b-station/v1/complete — NME支付完成通知
 *   GET  /wp-json/b-station/v1/return   — 用户支付后跳转（无需验签）
 *   GET  /wp-json/b-station/v1/cancel   — 用户取消支付（无需验签）
 *   GET  /wp-json/b-station/v1/query    — NME轮询订单状态（需验签）
 *   GET  /wp-json/b-station/v1/ping     — NME心跳探活（需验签）
 */
class OSB_Receiver_API
{
    public function __construct()
    {
        add_action('rest_api_init', [$this, 'register_routes']);
    }

    public function register_routes(): void
    {
        $ns = 'b-station/v1';
        register_rest_route($ns, '/order',    ['methods' => 'POST', 'callback' => [$this, 'handle_order'],    'permission_callback' => '__return_true']);
        register_rest_route($ns, '/complete', ['methods' => 'POST', 'callback' => [$this, 'handle_complete'], 'permission_callback' => '__return_true']);
        register_rest_route($ns, '/return',   ['methods' => 'GET',  'callback' => [$this, 'handle_return'],   'permission_callback' => '__return_true']);
        register_rest_route($ns, '/cancel',   ['methods' => 'GET',  'callback' => [$this, 'handle_cancel'],   'permission_callback' => '__return_true']);
        register_rest_route($ns, '/query',    ['methods' => 'GET',  'callback' => [$this, 'handle_query'],    'permission_callback' => '__return_true']);
        register_rest_route($ns, '/ping',     ['methods' => 'GET',  'callback' => [$this, 'handle_ping'],     'permission_callback' => '__return_true']);
    }

    private function get_settings(): array
    {
        return get_option('woocommerce_order_sync_base_settings', []);
    }

    // ════════════════════════════════════════════════════════
    // 统一 HMAC Header 验签
    //
    // NME 发送：
    //   X-Api-Key:   b_api_key（bk_live_xxx，明文标识符）
    //   X-Timestamp: Unix 秒
    //   X-Signature: HMAC-SHA256(api_key+"\n"+ts+"\n"+sha256(body), b_shared_secret)
    //
    // B站存储（插件设置）：
    //   b_api_key       — 明文，对比 X-Api-Key，确认来源
    //   b_shared_secret — 密钥，用于重算签名并比对
    //
    // 安全性：
    //   ① API Key 比对防止其他端点的请求混入
    //   ② 时间戳窗口 ±300s 防重放
    //   ③ HMAC 比对防篡改
    //   ④ hash_equals 防时序攻击
    // ════════════════════════════════════════════════════════

    private function verify_hmac(WP_REST_Request $request): bool
    {
        $settings     = $this->get_settings();
        $storedKey    = $settings['b_api_key']       ?? '';
        $sharedSecret = $settings['b_shared_secret'] ?? '';

        if (!$storedKey || !$sharedSecret) return false;

        $incomingKey = $request->get_header('x_api_key')   ?: ($_SERVER['HTTP_X_API_KEY']   ?? '');
        $ts          = (int)($request->get_header('x_timestamp') ?: ($_SERVER['HTTP_X_TIMESTAMP'] ?? 0));
        $sig         = $request->get_header('x_signature') ?: ($_SERVER['HTTP_X_SIGNATURE'] ?? '');
        $body        = $request->get_body();

        if (!$incomingKey || !$ts || !$sig) return false;

        // ① 标识符比对（防止跨端点请求）
        if (!hash_equals($storedKey, $incomingKey)) return false;

        // ② 时间戳窗口（±300s）
        if (abs(time() - $ts) > 300) return false;

        // ③ HMAC 重算比对
        $payload  = $incomingKey . "\n" . $ts . "\n" . hash('sha256', $body);
        $expected = hash_hmac('sha256', $payload, $sharedSecret);
        return hash_equals($expected, $sig);
    }

    // ════════════════════════════════════════════════════════
    // GET /wp-json/b-station/v1/ping — 心跳探活
    // ════════════════════════════════════════════════════════

    public function handle_ping(WP_REST_Request $request): WP_REST_Response
    {
        if (!$this->verify_hmac($request)) {
            return new WP_REST_Response(['error' => '认证失败'], 401);
        }
        return new WP_REST_Response([
            'status'  => 'ok',
            'ts'      => time(),
            'version' => defined('OSB_VERSION') ? OSB_VERSION : '7.1',
        ], 200);
    }

    // ════════════════════════════════════════════════════════
    // POST /wp-json/b-station/v1/order — NME建单通知
    // ════════════════════════════════════════════════════════

    public function handle_order(WP_REST_Request $request): WP_REST_Response
    {
        if (!$this->verify_hmac($request)) {
            return new WP_REST_Response(['error' => '认证失败'], 401);
        }

        $data      = json_decode($request->get_body(), true) ?? [];
        $aOrderId  = (string)($data['a_order_id'] ?? '');
        $bOrderId  = (string)($data['b_order_id'] ?? '');
        $amount    = (float)($data['amount']       ?? 0);
        $currency  = strtoupper($data['currency']  ?? 'USD');
        $email     = $data['email']                ?? '';
        $billing   = $data['billing']              ?? [];
        $clientIp  = $data['client_ip']            ?? '';
        $userAgent = $data['user_agent']            ?? '';

        if (!$aOrderId || !$amount) {
            return new WP_REST_Response(['error' => '缺少必填参数 a_order_id / amount'], 400);
        }

        // 幂等：同一 a_order_id 已建单则直接返回
        $existing = wc_get_orders([
            'meta_key'   => '_osb_a_order_id',
            'meta_value' => $aOrderId,
            'return'     => 'objects',
            'limit'      => 1,
        ]);
        if (!empty($existing)) {
            return new WP_REST_Response(['status' => 'exists', 'b_wc_order_id' => $existing[0]->get_id()], 200);
        }

        $productId = $this->get_next_product_id();
        $order     = wc_create_order();
        $order->set_currency($currency);

        if ($productId) {
            $product = wc_get_product($productId);
            if ($product) {
                $order->add_product($product, 1, ['subtotal' => $amount, 'total' => $amount]);
            }
        }
        $order->set_total($amount);

        if (!empty($billing)) {
            $nameParts = explode(' ', trim($billing['name'] ?? ''), 2);
            $order->set_billing_first_name($nameParts[0] ?? '');
            $order->set_billing_last_name($nameParts[1]  ?? '');
            $order->set_billing_email($email ?: ($billing['email'] ?? ''));
            $order->set_billing_phone($billing['phone']    ?? '');
            $order->set_billing_address_1($billing['address']  ?? '');
            $order->set_billing_address_2($billing['address2'] ?? '');
            $order->set_billing_city($billing['city']     ?? '');
            $order->set_billing_state($billing['state']   ?? '');
            $order->set_billing_postcode($billing['postcode'] ?? '');
            $order->set_billing_country($billing['country']  ?? '');
        }

        $order->update_meta_data('_osb_a_order_id', $aOrderId);
        $order->update_meta_data('_osb_b_order_id', $bOrderId);
        $order->update_meta_data('_osb_product_id', (string)($productId ?? ''));
        if ($clientIp)  $order->update_meta_data('_osb_client_ip', $clientIp);
        if ($userAgent) $order->update_meta_data('_osb_user_agent', mb_substr($userAgent, 0, 300));

        $order->set_payment_method('order_sync_base');
        $order->set_status('pending', 'NME建单通知，等待支付完成。');
        $order->save();

        return new WP_REST_Response(['status' => 'ok', 'b_wc_order_id' => $order->get_id()], 200);
    }

    // ════════════════════════════════════════════════════════
    // POST /wp-json/b-station/v1/complete — NME支付完成通知
    //
    // v7.1 变更：
    //   旧：单层 HMAC Header + body.sign 二次验签（两层）
    //   新：单层 HMAC Header（去掉 body.sign，Header 已足够安全）
    // ════════════════════════════════════════════════════════

    public function handle_complete(WP_REST_Request $request): WP_REST_Response
    {
        if (!$this->verify_hmac($request)) {
            return new WP_REST_Response(['error' => '认证失败'], 401);
        }

        $data     = json_decode($request->get_body(), true) ?? [];
        $aOrderId = (string)($data['a_order_id']     ?? '');
        $txId     = (string)($data['transaction_id'] ?? '');

        $orders = wc_get_orders([
            'meta_key'   => '_osb_a_order_id',
            'meta_value' => $aOrderId,
            'return'     => 'objects',
            'limit'      => 1,
        ]);
        if (empty($orders)) {
            return new WP_REST_Response(['error' => '订单不存在'], 404);
        }

        $order = $orders[0];
        if (in_array($order->get_status(), ['completed', 'processing'])) {
            return new WP_REST_Response(['status' => 'already_done'], 200);
        }

        // 分布式幂等锁（30s TTL）
        $lockKey = 'osb_complete_' . $order->get_id();
        if (get_transient($lockKey)) {
            return new WP_REST_Response(['status' => 'processing'], 200);
        }
        set_transient($lockKey, 1, 30);

        try {
            // 再次读取防止并发
            $order = wc_get_order($order->get_id());
            if (in_array($order->get_status(), ['completed', 'processing'])) {
                return new WP_REST_Response(['status' => 'already_done'], 200);
            }
            $order->payment_complete($txId);
            wc_reduce_stock_levels($order->get_id());
            $order->update_meta_data('_osb_transaction_id', $txId);
            $order->save();
        } finally {
            delete_transient($lockKey);
        }

        return new WP_REST_Response(['status' => 'ok'], 200);
    }

    // ════════════════════════════════════════════════════════
    // GET /wp-json/b-station/v1/return — 用户支付后跳转（无需验签）
    // ════════════════════════════════════════════════════════

    public function handle_return(WP_REST_Request $request): void
    {
        $token    = sanitize_text_field($request->get_param('token') ?? '');
        if (!$token) { wp_redirect(home_url('/')); exit; }

        $settings  = $this->get_settings();
        $nmeUrl    = rtrim($settings['nme_url'] ?? '', '/');
        $returnUrl = '';
        $aOrderId  = '';

        if ($nmeUrl) {
            $resp = wp_remote_get($nmeUrl . '/api/gateway/status/' . urlencode($token), ['timeout' => 10]);
            if (!is_wp_error($resp) && wp_remote_retrieve_response_code($resp) === 200) {
                $raw       = json_decode(wp_remote_retrieve_body($resp), true);
                $data      = $raw['data'] ?? $raw;
                $returnUrl = $data['return_url'] ?? '';
                if (!$returnUrl) {
                    $returnUrl = $data['checkout_url'] ?? '';
                }
                $aOrderId  = $data['a_order_id'] ?? '';
            }
        }

        // 支付成功页进入时，主动补发一次 completed（幂等，NME侧会做状态保护）
        if ($token) {
            $this->notify_nme_status($token, 'completed');
        }

        $bOrder = null;
        if ($aOrderId) {
            $orders = wc_get_orders([
                'meta_key'   => '_osb_a_order_id',
                'meta_value' => $aOrderId,
                'return'     => 'objects',
                'limit'      => 1,
            ]);
            if (!empty($orders)) $bOrder = $orders[0];
        }

        $this->render_thanks_page($returnUrl, $bOrder);
        exit;
    }

    // ════════════════════════════════════════════════════════
    // GET /wp-json/b-station/v1/cancel — 用户取消支付（无需验签）
    // ════════════════════════════════════════════════════════

    public function handle_cancel(WP_REST_Request $request): void
    {
        $token     = sanitize_text_field($request->get_param('token') ?? '');
        $settings  = $this->get_settings();
        $nmeUrl    = rtrim($settings['nme_url'] ?? '', '/');
        $returnUrl = home_url('/');
        $aOrderId  = '';

        if ($token && $nmeUrl) {
            // 主动告知 NME：用户在 B站手动返回/取消支付
            $this->notify_nme_status($token, 'abandoned');

            $resp = wp_remote_get($nmeUrl . '/api/gateway/status/' . urlencode($token), ['timeout' => 10]);
            if (!is_wp_error($resp) && wp_remote_retrieve_response_code($resp) === 200) {
                $raw       = json_decode(wp_remote_retrieve_body($resp), true);
                $data      = $raw['data'] ?? $raw;
                $returnUrl = $data['return_url'] ?? home_url('/');
                if (!$returnUrl) {
                    $returnUrl = $data['checkout_url'] ?? home_url('/');
                }
                $aOrderId  = $data['a_order_id'] ?? '';
            }
        }

        if ($aOrderId) {
            $orders = wc_get_orders([
                'meta_key'   => '_osb_a_order_id',
                'meta_value' => $aOrderId,
                'return'     => 'objects',
                'limit'      => 1,
            ]);
            if (!empty($orders)) {
                $order = $orders[0];
                if (!in_array($order->get_status(), ['completed', 'processing'])) {
                    $order->update_status('cancelled', '用户取消支付。');
                }
            }
        }

        wp_redirect($returnUrl);
        exit;
    }

    // ════════════════════════════════════════════════════════
    // GET /wp-json/b-station/v1/query — NME轮询订单状态（需验签）
    // ════════════════════════════════════════════════════════

    public function handle_query(WP_REST_Request $request): WP_REST_Response
    {
        if (!$this->verify_hmac($request)) {
            return new WP_REST_Response(['error' => '认证失败'], 401);
        }

        $aOrderId = sanitize_text_field($request->get_param('a_order_id') ?? '');
        if (!$aOrderId) return new WP_REST_Response(['error' => '缺少 a_order_id'], 400);

        $orders = wc_get_orders([
            'meta_key'   => '_osb_a_order_id',
            'meta_value' => $aOrderId,
            'return'     => 'objects',
            'limit'      => 1,
        ]);
        if (empty($orders)) return new WP_REST_Response(['status' => 'not_found'], 404);

        $order  = $orders[0];
        $status = $order->get_status();
        return new WP_REST_Response([
            'status'        => $status,
            'b_wc_order_id' => $order->get_id(),
            'completed'     => in_array($status, ['completed', 'processing']),
        ], 200);
    }

    // ── 产品轮询池（逻辑不变）────────────────────────────────

    private function get_next_product_id(): ?int
    {
        $pool  = get_option('osb_product_pool', []);
        $pool  = array_values(array_filter($pool, fn($v) => (int)$v > 0));
        if (empty($pool)) return null;

        $count     = count($pool);
        $index     = (int) get_option('osb_product_poll_index', 0) % $count;
        $productId = (int) $pool[$index];
        update_option('osb_product_poll_index', ($index + 1) % $count);

        return $productId > 0 ? $productId : null;
    }

    // ── 支付成功页（样式不变）────────────────────────────────

    private function render_thanks_page(string $returnUrl, ?WC_Order $order = null): void
    {
        if (!headers_sent()) {
            header_remove('Content-Type');
            header('Content-Type: text/html; charset=UTF-8');
        }
        $siteName   = get_bloginfo('name');
        $safeReturn = esc_url($returnUrl ?: home_url('/'));
        ?>
<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>支付成功 — <?php echo esc_html($siteName); ?></title>
<style>
  *{box-sizing:border-box;margin:0;padding:0}
  body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;
    background:linear-gradient(135deg,#667eea,#764ba2);min-height:100vh;
    display:flex;align-items:center;justify-content:center;padding:24px}
  .card{background:#fff;border-radius:20px;box-shadow:0 20px 60px rgba(0,0,0,.2);
    padding:52px 44px 44px;max-width:420px;width:100%;text-align:center}
  .check{width:80px;height:80px;background:linear-gradient(135deg,#00b894,#00cec9);
    border-radius:50%;display:flex;align-items:center;justify-content:center;
    margin:0 auto 24px;animation:pop .5s cubic-bezier(.34,1.56,.64,1) both}
  @keyframes pop{0%{transform:scale(0);opacity:0}100%{transform:scale(1);opacity:1}}
  .check svg{width:40px;height:40px;stroke:#fff;stroke-width:3;fill:none;stroke-linecap:round;stroke-linejoin:round}
  h1{font-size:24px;font-weight:700;color:#1a1a1a;margin-bottom:10px}
  .sub{font-size:14px;color:#6b7280;line-height:1.65;margin-bottom:32px}
  .bar-track{height:4px;background:#e5e7eb;border-radius:99px;overflow:hidden;margin-bottom:24px}
  .bar{height:100%;background:linear-gradient(90deg,#667eea,#764ba2);border-radius:99px;width:100%;transition:width 3s linear}
  .btn{display:block;width:100%;padding:14px;border-radius:12px;font-size:15px;font-weight:600;
    color:#fff;background:linear-gradient(135deg,#667eea,#764ba2);border:none;cursor:pointer;text-decoration:none}
</style>
</head>
<body>
<div class="card">
  <div class="check"><svg viewBox="0 0 24 24"><polyline points="20 6 9 17 4 12"/></svg></div>
  <h1>支付成功！</h1>
  <p class="sub">订单已确认，感谢您的购买。<br>正在跳转到订单确认页面…</p>
  <div class="bar-track"><div class="bar" id="bar"></div></div>
  <a href="<?php echo $safeReturn; ?>" class="btn" id="btn">立即跳转 →</a>
</div>
<script>
(function(){
  var url=<?php echo wp_json_encode($returnUrl ?: home_url('/')); ?>;
  var n=3;
  setTimeout(function(){document.getElementById('bar').style.width='0%'},50);
  var t=setInterval(function(){n--;if(n<=0){clearInterval(t);location.href=url;}},1000);
  document.getElementById('btn').onclick=function(e){e.preventDefault();clearInterval(t);location.href=url;};
}());
</script>
</body></html>
        <?php
    }

    // 主动通知 NME 订单状态（用于手动返回/取消）
    private function notify_nme_status(string $payToken, string $status): void
    {
        $settings     = $this->get_settings();
        $nmeUrl       = rtrim($settings['nme_url'] ?? '', '/');
        $apiKey       = $settings['b_api_key'] ?? '';
        $sharedSecret = $settings['b_shared_secret'] ?? '';
        if (!$nmeUrl || !$apiKey || !$sharedSecret || !$payToken) {
            return;
        }

        $payload   = ['pay_token' => $payToken, 'status' => $status];
        $body      = wp_json_encode($payload);
        $ts        = time();
        $signInput = $apiKey . "\n" . $ts . "\n" . hash('sha256', $body);
        $sig       = hash_hmac('sha256', $signInput, $sharedSecret);

        wp_remote_post($nmeUrl . '/api/gateway/callback', [
            'timeout' => 8,
            'headers' => [
                'Content-Type' => 'application/json',
                'X-Api-Key'    => $apiKey,
                'X-Timestamp'  => (string)$ts,
                'X-Signature'  => $sig,
            ],
            'body' => $body,
        ]);
    }
}
