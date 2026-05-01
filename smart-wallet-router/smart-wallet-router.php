<?php
/**
 * Plugin Name: Smart Wallet Router
 * Description: A站PayPal支付插件 v7.1 — 统一HMAC签名，Secret不出网，支持一码配置串
 * Version: 7.1
 * Requires Plugins: woocommerce
 */
defined('ABSPATH') || exit;

// ── 加载网关类 ──────────────────────────────────────────────

add_filter('woocommerce_payment_gateways', function ($gateways) {
    require_once __DIR__ . '/includes/class-gateway-paypal.php';
    $gateways[] = 'Smart_Wallet_Router_Gateway';
    return $gateways;
});

// ── REST：接收 NME → A站 支付完成回调 ──────────────────────
// PayPal 和 Stripe 插件共用同一回调端点 /wp-json/nme/v1/callback
// 如果 Stripe 插件已注册，PayPal 插件跳过注册，共用 gca_handle_nme_callback

add_action('rest_api_init', function () {
    if (function_exists('gca_handle_nme_callback')) {
        // Stripe 插件已注册，共用其回调入口，无需重复注册
        return;
    }

    register_rest_route('nme/v1', '/callback', [
        'methods'             => 'POST',
        'callback'            => 'swr_handle_nme_callback',
        'permission_callback' => '__return_true',
    ]);
});

/**
 * NME → A站 支付完成回调 v7.1（PayPal 插件独立版）
 *
 * 仅在 Stripe 插件未激活时生效。
 * 验签逻辑与 gca_handle_nme_callback 完全一致。
 * 优先读取 Stripe 插件的 api_secret，其次读取 PayPal 插件自身的。
 *
 * payload   = api_key + "\n" + timestamp + "\n" + sha256(raw_body)
 * signature = HMAC-SHA256(payload, api_secret)
 * Headers:  X-Api-Key / X-Timestamp / X-Signature（±300s 时间窗口）
 */
function swr_handle_nme_callback(WP_REST_Request $request): WP_REST_Response
{
    // 优先读 Stripe 插件设置（两个插件共用一个回调地址，共用一个 secret）
    $settings  = get_option('woocommerce_global_card_adapter_settings', []);
    $apiSecret = $settings['api_secret'] ?? '';
    if (!$apiSecret) {
        $settings  = get_option('woocommerce_smart_wallet_router_settings', []);
        $apiSecret = $settings['api_secret'] ?? '';
    }

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

    $order = wc_get_order((int) $orderId);
    if (!$order) {
        return new WP_REST_Response(['status' => 'error', 'message' => '订单不存在'], 404);
    }
    switch ($status) {
        case 'paid':
            if (!in_array($order->get_status(), ['processing', 'completed'])) {
                $order->update_status('processing', 'NME支付成功回调，自动改为Processing。');
            }
            break;
        case 'processing':
            $order->add_order_note('NME状态：processing（支付进行中）');
            break;
        case 'abandoned':
            if (!in_array($order->get_status(), ['processing', 'completed'])) {
                $order->update_status('cancelled', 'NME回调：用户放弃/超时未支付。');
            }
            break;
        case 'failed':
            if (!in_array($order->get_status(), ['processing', 'completed'])) {
                $order->update_status('failed', 'NME回调：支付失败。');
            }
            break;
        default:
            return new WP_REST_Response(['status' => 'ignored'], 200);
    }
    $order->save();

    return new WP_REST_Response(['status' => 'ok'], 200);
}
