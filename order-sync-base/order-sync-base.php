<?php
/**
 * Plugin Name: Order Sync Base
 * Description: B站支付中转插件 v7.1 — 统一HMAC签名鉴权，Secret不传输，支持一码配置串
 * Version: 7.1
 * Requires Plugins: woocommerce
 */
defined('ABSPATH') || exit;

define('OSB_VERSION', '7.1');

add_filter('woocommerce_payment_gateways', function ($gateways) {
    require_once __DIR__ . '/includes/class-gateway-b.php';
    $gateways[] = 'Order_Sync_Base_Gateway';
    return $gateways;
});

add_action('plugins_loaded', function () {
    require_once __DIR__ . '/includes/class-receiver-api.php';
    new OSB_Receiver_API();
});
