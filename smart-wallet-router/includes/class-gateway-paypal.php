<?php
defined('ABSPATH') || exit;

/**
 * Smart_Wallet_Router_Gateway v7.1
 *
 * A站 PayPal 支付网关，逻辑与 Global_Card_Adapter_Gateway（Stripe）完全对称。
 *
 * 认证变更（v7.1）：
 *   旧：X-API-Key + X-API-Secret（明文 secret 传输）
 *   新：X-Api-Key + X-Timestamp + X-Signature（HMAC，secret 不出网）
 *
 * 配置字段（3个，与 Stripe 插件一致）：
 *   nme_url    — NME 服务地址
 *   api_key    — ak_cb_xxx（A站端点标识符）
 *   api_secret — whsec_xxx（HMAC签名密钥，同时用于回调验签）
 *
 * 一码配置串格式（从 NME 后台复制）：
 *   Base64({ "v":2, "type":"a", "nme":"https://...", "ak":"ak_cb_xxx", "whsec":"whsec_xxx" })
 */
class Smart_Wallet_Router_Gateway extends WC_Payment_Gateway
{
    public string $api_key    = '';
    public string $api_secret = '';
    public string $nme_url    = '';

    public function __construct()
    {
        $this->id                 = 'smart_wallet_router';
        $this->method_title       = 'Smart Wallet Router (PayPal via NME)';
        $this->method_description = 'A站PayPal支付网关 v7.1。统一HMAC签名，Secret不出网，支持一码配置串。';
        $this->has_fields         = false;
        $this->supports           = ['products', 'cart_checkout_blocks'];

        $this->init_form_fields();
        $this->init_settings();

        $this->title      = $this->get_option('title', 'PayPal');
        $this->description= $this->get_option('description', 'Pay securely via PayPal.');
        $this->enabled    = $this->get_option('enabled', 'yes');
        $this->nme_url    = rtrim($this->get_option('nme_url', ''), '/');
        $this->api_key    = $this->get_option('api_key', '');
        $this->api_secret = $this->get_option('api_secret', '');

        add_action('woocommerce_update_options_payment_gateways_' . $this->id, [$this, 'process_admin_options']);
        if (is_admin()) {
            add_action('admin_footer', [$this, 'inject_admin_scripts']);
        }
    }

    public function is_available(): bool { return $this->enabled === 'yes'; }

    public function init_form_fields(): void
    {
        $callback_url = rest_url('nme/v1/callback');

        $this->form_fields = [
            'enabled' => [
                'title'   => '启用/禁用',
                'type'    => 'checkbox',
                'label'   => '启用 Smart Wallet Router (PayPal)',
                'default' => 'yes',
            ],
            'title' => [
                'title'   => '支付名称',
                'type'    => 'text',
                'default' => 'PayPal',
            ],
            'description' => [
                'title'   => '描述',
                'type'    => 'textarea',
                'default' => 'Pay securely via PayPal.',
            ],

            // ── 一码配置串（与 Stripe 插件字段名完全一致，可共用同一配置串）──
            'config_string_info' => [
                'title'       => '⚡ 一码配置串（推荐）',
                'type'        => 'title',
                'description' => $this->render_config_string_input(),
            ],

            'nme_info' => [
                'title'       => '📌 NME 回调地址',
                'type'        => 'title',
                'description' =>
                    '<div style="background:#e8f4fd;border:1px solid #0073aa;border-radius:4px;padding:12px;">'
                  . '<strong>将此地址填入 NME 后台 → A站端点 → URL：</strong><br>'
                  . '<div style="display:flex;align-items:center;gap:8px;margin-top:6px;">'
                  . '<code id="swr_cb_url" style="flex:1;padding:6px 8px;background:#fff;border:1px solid #8c8f94;border-radius:3px;font-size:13px;">'
                  . esc_html($callback_url) . '</code>'
                  . '<button type="button" id="swr_copy_cb_btn" class="button button-secondary">复制</button>'
                  . '</div></div>',
            ],

            'manual_info' => [
                'title'       => '🔑 手动填写',
                'type'        => 'title',
                'description' => '<div style="background:#fff8e1;border:1px solid #f0b429;border-radius:4px;padding:8px 12px;font-size:13px;">粘贴配置串后，下方字段将自动填充。也可手动填写。</div>',
            ],
            'nme_url' => [
                'title'       => 'NME 地址',
                'type'        => 'text',
                'placeholder' => 'https://nme.yourdomain.com',
            ],
            'api_key' => [
                'title'       => 'API Key',
                'type'        => 'text',
                'description' => 'NME后台 → A站端点 → 复制 <code>ak_cb_xxx</code>',
                'placeholder' => 'ak_cb_xxx',
            ],
            'api_secret' => [
                'title'       => 'API Secret / Webhook Secret',
                'type'        => 'password',
                'description' => '同时用于：① A站→NME 请求签名；② NME→A站 回调验签。对应配置串 <code>whsec</code> 字段。',
                'placeholder' => 'whsec_xxx',
            ],
            'channel' => [
                'title'   => '渠道标识',
                'type'    => 'text',
                'default' => 'a',
            ],
        ];
    }

    private function render_config_string_input(): string
    {
        return '<div style="background:#f0f7f0;border:1px solid #4CAF50;border-radius:4px;padding:14px;">'
             . '<p style="margin:0 0 10px;font-size:13px;color:#333;">从 <strong>NME 后台 → A站端点 → 复制配置串</strong>，粘贴到下方，所有字段自动填充。</p>'
             . '<div style="display:flex;gap:8px;align-items:flex-start;">'
             . '<textarea id="swr_config_string" rows="3" style="flex:1;font-family:monospace;font-size:12px;padding:6px;border:1px solid #8c8f94;border-radius:3px;resize:vertical;" placeholder="粘贴配置串（Base64字符串）..."></textarea>'
             . '<button type="button" id="swr_parse_config_btn" class="button button-primary" style="white-space:nowrap;">解析填充 →</button>'
             . '</div>'
             . '<p id="swr_config_status" style="margin:6px 0 0;font-size:12px;color:#666;"></p>'
             . '</div>';
    }

    // ── 下单 ──────────────────────────────────────────────

    public function process_payment($order_id): array
    {
        foreach (['nme_url' => 'NME地址', 'api_key' => 'API Key', 'api_secret' => 'API Secret'] as $field => $label) {
            if (empty($this->$field)) {
                wc_add_notice("支付配置错误：请填写{$label}。", 'error');
                return ['result' => 'failure'];
            }
        }

        $order   = wc_get_order($order_id);
        $billing = $this->get_billing($order);
        $payload = [
            'a_order_id'     => (string) $order_id,
            'amount'         => number_format((float) $order->get_total(), 2, '.', ''),
            'currency'       => $order->get_currency(),
            'payment_method' => 'paypal',
            'email'          => $order->get_billing_email(),
            'billing'        => $billing,
            'client_ip'      => $this->get_real_client_ip(),
            'user_agent'     => $_SERVER['HTTP_USER_AGENT'] ?? '',
            'return_url'     => $order->get_checkout_order_received_url(),
            'channel'        => $this->get_option('channel', 'a'),
            'nonce'          => bin2hex(random_bytes(8)),
        ];

        $bodyJson  = wp_json_encode($payload);
        $timestamp = time();
        $headers   = $this->build_hmac_headers($this->api_key, $this->api_secret, $bodyJson, $timestamp);

        $response = wp_remote_post($this->nme_url . '/api/gateway/order', [
            'headers' => array_merge($headers, ['Content-Type' => 'application/json', 'Accept' => 'application/json']),
            'body'    => $bodyJson,
            'timeout' => 30,
        ]);

        if (is_wp_error($response)) {
            wc_add_notice('支付初始化失败：' . $response->get_error_message(), 'error');
            return ['result' => 'failure'];
        }

        $body = json_decode(wp_remote_retrieve_body($response), true);
        $code = (int) wp_remote_retrieve_response_code($response);

        if ($code !== 200 || empty($body['payment_url'])) {
            $errMsg = $body['error'] ?? ('NME返回 HTTP ' . $code);
            wc_add_notice('支付请求失败：' . esc_html($errMsg), 'error');
            return ['result' => 'failure'];
        }

        $order->update_meta_data('_swr_nme_order_id', $body['order_id'] ?? '');
        $order->update_status('pending', '已提交NME（PayPal），等待支付完成。');
        $order->save();
        WC()->cart->empty_cart();

        return ['result' => 'success', 'redirect' => esc_url_raw($body['payment_url'])];
    }

    // ── HMAC 签名构造 ────────────────────────────────────

    private function build_hmac_headers(string $apiKey, string $secret, string $body, int $ts): array
    {
        $payload   = $apiKey . "\n" . $ts . "\n" . hash('sha256', $body);
        $signature = hash_hmac('sha256', $payload, $secret);
        return [
            'X-Api-Key'   => $apiKey,
            'X-Timestamp' => (string) $ts,
            'X-Signature' => $signature,
        ];
    }

    // ── 工具方法 ─────────────────────────────────────────

    private function get_billing(WC_Order $order): array
    {
        return [
            'name'     => trim($order->get_billing_first_name() . ' ' . $order->get_billing_last_name()),
            'email'    => $order->get_billing_email(),
            'phone'    => $order->get_billing_phone(),
            'address'  => $order->get_billing_address_1(),
            'address2' => $order->get_billing_address_2(),
            'city'     => $order->get_billing_city(),
            'state'    => $order->get_billing_state(),
            'postcode' => $order->get_billing_postcode(),
            'country'  => $order->get_billing_country(),
        ];
    }

    private function get_real_client_ip(): string
    {
        foreach (['HTTP_CF_CONNECTING_IP', 'HTTP_X_FORWARDED_FOR', 'HTTP_X_REAL_IP', 'REMOTE_ADDR'] as $key) {
            if (!empty($_SERVER[$key])) {
                $ip = trim(explode(',', $_SERVER[$key])[0]);
                if (filter_var($ip, FILTER_VALIDATE_IP)) return $ip;
            }
        }
        return '';
    }

    // ── 管理端 JS ────────────────────────────────────────

    public function inject_admin_scripts(): void
    {
        if (!isset($_GET['section']) || $_GET['section'] !== $this->id) return;
        ?>
        <script>
        jQuery(document).ready(function($) {

            $('#swr_copy_cb_btn').on('click', function() {
                var text = $('#swr_cb_url').text().trim();
                navigator.clipboard?.writeText(text).then(() => {
                    $(this).text('已复制 ✓');
                    setTimeout(() => $(this).text('复制'), 2000);
                });
            });

            $('#swr_parse_config_btn').on('click', function() {
                var raw = $('#swr_config_string').val().trim();
                if (!raw) {
                    $('#swr_config_status').css('color','#c00').text('请先粘贴配置串。');
                    return;
                }
                try {
                    var json = atob(raw);
                    var cfg  = JSON.parse(json);

                    if (cfg.type !== 'a') {
                        $('#swr_config_status').css('color','#c00').text('错误：这是 B站配置串，请使用 A站配置串。');
                        return;
                    }

                    if (cfg.nme)   $('#woocommerce_smart_wallet_router_nme_url').val(cfg.nme);
                    if (cfg.ak)    $('#woocommerce_smart_wallet_router_api_key').val(cfg.ak);
                    if (cfg.whsec) $('#woocommerce_smart_wallet_router_api_secret').val(cfg.whsec);

                    $('#swr_config_status').css('color','#4CAF50').text('✓ 配置串解析成功，字段已自动填充，请点击「保存更改」。');
                } catch(e) {
                    $('#swr_config_status').css('color','#c00').text('解析失败：配置串格式错误，请重新从NME后台复制。');
                }
            });

            $('#swr_config_string').on('paste', function() {
                setTimeout(function() { $('#swr_parse_config_btn').trigger('click'); }, 100);
            });
        });
        </script>
        <?php
    }
}
