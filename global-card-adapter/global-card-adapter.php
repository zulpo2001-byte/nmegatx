<?php
/**
 * Plugin Name: Global Card Adapter
 * Description: A站Stripe支付插件 v7.1 — 统一HMAC签名，Secret不出网，支持一码配置串
 * Version: 7.1
 * Requires Plugins: woocommerce
 */
defined('ABSPATH') || exit;

// ── 加载网关类 ──────────────────────────────────────────────

add_filter('woocommerce_payment_gateways', function ($gateways) {
    require_once __DIR__ . '/includes/class-gateway-stripe.php';
    $gateways[] = 'Global_Card_Adapter_Gateway';
    return $gateways;
});

add_action('woocommerce_blocks_loaded', function () {
    if (!class_exists('Automattic\WooCommerce\Blocks\Payments\Integrations\AbstractPaymentMethodType')) return;
    require_once __DIR__ . '/includes/class-gateway-stripe.php';
    add_action('woocommerce_blocks_payment_method_type_registration', function ($registry) {
        // Blocks 支持类可按需添加
    });
});

// ── REST：接收 NME → A站 支付完成回调 ──────────────────────
// POST /wp-json/nme/v1/callback

add_action('rest_api_init', function () {
    register_rest_route('nme/v1', '/callback', [
        'methods'             => 'POST',
        'callback'            => 'gca_handle_nme_callback',
        'permission_callback' => '__return_true',
    ]);
});

/**
 * NME → A站 支付完成回调 v7.1
 *
 * 验签方案（统一 HMAC Header）：
 *   payload   = api_key + "\n" + timestamp + "\n" + sha256(raw_body)
 *   signature = HMAC-SHA256(payload, api_secret)
 *   Headers:  X-Api-Key / X-Timestamp / X-Signature
 *
 * api_secret 从插件设置读取（即原 api_secret 字段，已合并 whsec 功能）
 * ±300s 时间戳窗口防重放
 */
function gca_handle_nme_callback(WP_REST_Request $request): WP_REST_Response
{
    // 优先读 Stripe 插件设置；PayPal 插件共用同一端点时也能验签
    $settings  = get_option('woocommerce_global_card_adapter_settings', []);
    $apiSecret = $settings['api_secret'] ?? '';

    if (!$apiSecret) {
        return new WP_REST_Response(['status' => 'error', 'message' => '插件未配置 API Secret'], 500);
    }

    // ── HMAC Header 验签 ──────────────────────────────────
    $apiKey = $request->get_header('x_api_key')   ?: ($_SERVER['HTTP_X_API_KEY']   ?? '');
    $ts     = (int)($request->get_header('x_timestamp') ?: ($_SERVER['HTTP_X_TIMESTAMP'] ?? 0));
    $sig    = $request->get_header('x_signature') ?: ($_SERVER['HTTP_X_SIGNATURE'] ?? '');
    $body   = $request->get_body();

    if (!$apiKey || !$ts || !$sig) {
        return new WP_REST_Response(['status' => 'error', 'message' => '缺少签名头'], 401);
    }
    if (abs(time() - $ts) > 300) {
        return new WP_REST_Response(['status' => 'error', 'message' => '请求已过期（±300s）'], 401);
    }

    $payload  = $apiKey . "\n" . $ts . "\n" . hash('sha256', $body);
    $expected = hash_hmac('sha256', $payload, $apiSecret);
    if (!hash_equals($expected, $sig)) {
        return new WP_REST_Response(['status' => 'error', 'message' => '签名验证失败'], 401);
    }

    // ── 业务逻辑 ──────────────────────────────────────────
    $data    = json_decode($body, true) ?? [];
    $orderId = (string)($data['order_id'] ?? '');
    $status  = $data['status'] ?? '';

    if ($status !== 'paid') {
        return new WP_REST_Response(['status' => 'ignored'], 200);
    }

    $order = wc_get_order((int) $orderId);
    if (!$order) {
        return new WP_REST_Response(['status' => 'error', 'message' => '订单不存在'], 404);
    }
    if (in_array($order->get_status(), ['processing', 'completed'])) {
        return new WP_REST_Response(['status' => 'already_done'], 200);
    }

    $order->update_status('processing', 'NME支付成功回调，自动改为Processing。');
    $order->save();

    return new WP_REST_Response(['status' => 'ok'], 200);
}
