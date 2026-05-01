<?php
defined('ABSPATH') || exit;

/**
 * Order_Sync_Base_Gateway v7.1
 *
 * 配置字段变更（v7.1）：
 *   旧：nme_url / b_api_key / b_api_secret（存哈希）/ shared_secret（3个密钥字段）
 *   新：nme_url / b_api_key / b_shared_secret（2个字段）
 *
 *   b_api_key      — bk_live_xxx，明文标识符，NME发来请求时放 X-Api-Key 头
 *   b_shared_secret — bsk_xxx，签名密钥，本地重算签名用，不传输
 *
 * 一码配置串格式（从 NME 后台复制）：
 *   Base64({ "v":2, "type":"b", "nme":"https://...", "bk":"bk_live_xxx", "bsk":"bsk_xxx" })
 */
class Order_Sync_Base_Gateway extends WC_Payment_Gateway
{
    public function __construct()
    {
        $this->id                 = 'order_sync_base';
        $this->method_title       = 'Order Sync Base';
        $this->method_description = 'B站支付中转插件 v7.1。统一HMAC签名鉴权，Secret不传输，支持一码配置串。';
        $this->has_fields         = false;
        $this->supports           = ['products'];

        $this->init_form_fields();
        $this->init_settings();

        $this->title   = $this->get_option('title', 'Order Sync Base');
        $this->enabled = $this->get_option('enabled', 'yes');

        add_action('woocommerce_update_options_payment_gateways_' . $this->id, [$this, 'process_admin_options']);
        add_action('woocommerce_update_options_payment_gateways_' . $this->id, [$this, 'save_product_pool']);
        add_action('wp_ajax_osb_reset_poll_index', [$this, 'ajax_reset_poll_index']);
        add_action('admin_enqueue_scripts', [$this, 'enqueue_admin_scripts']);
        add_action('admin_footer', [$this, 'inject_admin_scripts']);
    }

    public function enqueue_admin_scripts(): void
    {
        if (isset($_GET['section']) && $_GET['section'] === $this->id) {
            wp_enqueue_script('wc-enhanced-select');
            wp_enqueue_style('woocommerce_admin_styles');
        }
    }

    public function generate_osb_title_html($key, $data): string
    {
        $html  = '</table>';
        $html .= '<h3 class="wc-settings-sub-title" style="margin-top:20px;font-weight:600;">' . wp_kses_post($data['title']) . '</h3>';
        if (!empty($data['custom_html'])) $html .= $data['custom_html'];
        $html .= '<table class="form-table">';
        return $html;
    }

    public function init_form_fields(): void
    {
        $order_url = home_url('/index.php/wp-json/b-station/v1/order');
        $ping_url  = home_url('/index.php/wp-json/b-station/v1/ping');

        $this->form_fields = [
            'enabled' => ['title' => '启用', 'type' => 'checkbox', 'label' => '启用 Order Sync Base', 'default' => 'yes'],
            'title'   => ['title' => '名称', 'type' => 'text', 'default' => 'Order Sync Base'],

            // ── 一码配置串（最顶部）────────────────────────
            'config_string_section' => [
                'type'        => 'osb_title',
                'title'       => '⚡ Step 0：一码配置串（推荐）',
                'custom_html' => $this->render_config_string_ui(),
            ],

            // ── Step 1：端点地址 ──────────────────────────
            'endpoint_info' => [
                'type'        => 'osb_title',
                'title'       => '📌 Step 1：将建单端点填入 NME 后台',
                'custom_html' => $this->render_endpoint_info($order_url, $ping_url),
            ],

            // ── Step 2：NME 鉴权（2个字段）───────────────
            'nme_auth_info' => [
                'type'        => 'osb_title',
                'title'       => '🔑 Step 2：NME 鉴权配置',
                'custom_html' =>
                    '<div style="background:#fff8e1;border:1px solid #f0b429;border-radius:4px;padding:10px 14px;margin-bottom:15px;">'
                  . '在NME后台「Webhook端点 → B站端点」创建后，复制配置串（Step 0 自动填充），或手动填入下方两个字段。'
                  . '</div>',
            ],
            'nme_url' => [
                'title'       => 'NME 地址',
                'type'        => 'text',
                'description' => '例如 https://nme.yourdomain.com',
                'placeholder' => 'https://nme.yourdomain.com',
            ],
            'b_api_key' => [
                'title'       => 'API Key（标识符）',
                'type'        => 'text',
                'description' => 'NME后台 → B站端点 → 复制 <code>bk_live_xxx</code>（明文，用于 X-Api-Key 头）',
                'placeholder' => 'bk_live_xxx',
            ],
            'b_shared_secret' => [
                'title'       => 'Shared Secret（签名密钥）',
                'type'        => 'password',
                'description' => 'NME后台 → B站端点 → 复制 <code>bsk_xxx</code>（本地验签用，不传输）',
                'placeholder' => 'bsk_xxx',
            ],

            // ── Step 3：产品ID轮询池 ──────────────────────
            'product_pool_info' => [
                'type'        => 'osb_title',
                'title'       => '🔄 Step 3：产品ID轮询池（最多20个）',
                'custom_html' => $this->render_product_pool_ui(),
            ],
        ];
    }

    private function render_config_string_ui(): string
    {
        return '<div style="background:#f0f7f0;border:1px solid #4CAF50;border-radius:4px;padding:14px;margin-bottom:15px;">'
             . '<p style="margin:0 0 10px;font-size:13px;color:#333;">从 <strong>NME 后台 → B站端点 → 复制配置串</strong>，粘贴到下方，Step 2 所有字段自动填充，无需手动复制密钥。</p>'
             . '<div style="display:flex;gap:8px;align-items:flex-start;">'
             . '<textarea id="osb_config_string" rows="3" style="flex:1;font-family:monospace;font-size:12px;padding:6px;border:1px solid #8c8f94;border-radius:3px;resize:vertical;" placeholder="粘贴 B站配置串（Base64字符串）..."></textarea>'
             . '<button type="button" id="osb_parse_config_btn" class="button button-primary" style="white-space:nowrap;">解析填充 →</button>'
             . '</div>'
             . '<p id="osb_config_status" style="margin:6px 0 0;font-size:12px;color:#666;"></p>'
             . '</div>';
    }

    private function render_endpoint_info(string $order_url, string $ping_url): string
    {
        $copy = fn(string $url, string $id) =>
            '<button type="button" class="osb-copy-btn button button-small" data-copy="' . esc_attr($url) . '" id="' . $id . '" style="margin-left:6px;white-space:nowrap;">复制</button>';

        return '<div style="background:#e8f4fd;border:1px solid #0073aa;border-radius:4px;padding:12px 14px;margin-bottom:15px;">'
             . '<strong>将建单端点填入 NME后台 → B站端点 → URL：</strong>'
             . '<div style="display:flex;align-items:center;gap:8px;margin:6px 0;">'
             . '<code id="osb_order_url" style="flex:1;padding:6px;background:#fff;border:1px solid #8c8f94;border-radius:3px;font-size:13px;word-break:break-all;">' . esc_html($order_url) . '</code>'
             . $copy($order_url, 'osb_copy_order')
             . '</div>'
             . '<small style="color:#555;">心跳探活：<code>' . esc_html($ping_url) . '</code></small>'
             . '</div>';
    }

    private function render_product_pool_ui(): string
    {
        $pool  = get_option('osb_product_pool', []);
        $pool  = array_values(array_filter($pool, fn($v) => (int)$v > 0));
        $count = count($pool);
        $index = (int) get_option('osb_product_poll_index', 0);
        $max   = 20;

        $options_html = '';
        foreach ($pool as $product_id) {
            $product = wc_get_product($product_id);
            if ($product) {
                $label = wp_kses_post(html_entity_decode($product->get_formatted_name(), ENT_QUOTES, get_bloginfo('charset')));
                $options_html .= '<option value="' . esc_attr($product_id) . '" selected>' . esc_html($label) . '</option>';
            }
        }

        $html  = '<div id="osb-product-pool-wrap" style="background:#fff;border:1px solid #ddd;border-radius:4px;padding:14px;">';
        $html .= '<p style="margin:0 0 10px;color:#555;font-size:13px;">搜索并选择B站中真实存在的产品（最多 ' . $max . ' 个），系统按顺序轮询。</p>';
        $html .= '<select class="osb-select2-search" multiple style="width:100%;max-width:800px;" id="osb_product_pool" name="osb_product_pool[]" data-placeholder="🔍 输入产品名称或 SKU...">'
               . $options_html . '</select>';
        $html .= '<div style="margin-top:12px;"><span id="osb-pool-count" style="color:#888;font-size:13px;">已选：<strong id="osb-count-num">' . $count . '</strong> / ' . $max . '</span></div>';
        $html .= '<p style="margin:15px 0 0;font-size:12px;color:#888;border-top:1px dashed #eee;padding-top:10px;">';
        $html .= '轮询指针：<strong id="osb-poll-index">' . $index . '</strong>&nbsp;&nbsp;';
        $html .= '<a href="#" id="osb-reset-index">↺ 重置到 0</a></p></div>';
        return $html;
    }

    public function save_product_pool(): void
    {
        $pool = [];
        if (!empty($_POST['osb_product_pool']) && is_array($_POST['osb_product_pool'])) {
            foreach ($_POST['osb_product_pool'] as $val) {
                $id = (int) $val;
                if ($id > 0) $pool[] = $id;
            }
        }
        update_option('osb_product_pool', array_values(array_slice(array_unique($pool), 0, 20)));
    }

    public function ajax_reset_poll_index(): void
    {
        check_ajax_referer('osb_reset', 'nonce');
        if (!current_user_can('manage_woocommerce')) wp_die('权限不足');
        update_option('osb_product_poll_index', 0);
        wp_send_json_success();
    }

    public function inject_admin_scripts(): void
    {
        if (!isset($_GET['section']) || $_GET['section'] !== $this->id) return;
        $nonce       = wp_create_nonce('search-products');
        $reset_nonce = wp_create_nonce('osb_reset');
        ?>
        <script>
        document.addEventListener("DOMContentLoaded", function() {

            // ── 复制端点地址 ──
            document.querySelectorAll(".osb-copy-btn").forEach(function(btn) {
                btn.addEventListener("click", function() {
                    var text = this.getAttribute("data-copy");
                    if (navigator.clipboard) {
                        navigator.clipboard.writeText(text).then(function() {
                            btn.textContent = "已复制 ✓";
                            setTimeout(function() { btn.textContent = "复制"; }, 2000);
                        });
                    }
                });
            });

            // ── 配置串解析自动填充 ──
            function parseOsbConfig() {
                var raw = document.getElementById('osb_config_string').value.trim();
                var statusEl = document.getElementById('osb_config_status');
                if (!raw) {
                    statusEl.style.color = '#c00';
                    statusEl.textContent = '请先粘贴配置串。';
                    return;
                }
                try {
                    var json = atob(raw);
                    var cfg  = JSON.parse(json);

                    if (cfg.type !== 'b') {
                        statusEl.style.color = '#c00';
                        statusEl.textContent = '错误：这是 A站配置串，请使用 B站配置串。';
                        return;
                    }

                    if (cfg.nme) document.getElementById('woocommerce_order_sync_base_nme_url').value = cfg.nme;
                    if (cfg.bk)  document.getElementById('woocommerce_order_sync_base_b_api_key').value = cfg.bk;
                    if (cfg.bsk) document.getElementById('woocommerce_order_sync_base_b_shared_secret').value = cfg.bsk;

                    statusEl.style.color = '#4CAF50';
                    statusEl.textContent = '✓ 配置串解析成功，字段已自动填充，请点击「保存更改」。';
                } catch(e) {
                    statusEl.style.color = '#c00';
                    statusEl.textContent = '解析失败：配置串格式错误，请重新从NME后台复制。';
                }
            }

            var parseBtn = document.getElementById('osb_parse_config_btn');
            if (parseBtn) parseBtn.addEventListener('click', parseOsbConfig);

            var configInput = document.getElementById('osb_config_string');
            if (configInput) {
                configInput.addEventListener('paste', function() {
                    setTimeout(parseOsbConfig, 100);
                });
            }
        });

        jQuery(document).ready(function($) {
            var MAX = 20, $sel = $('#osb_product_pool');
            if ($.fn.selectWoo) {
                $sel.selectWoo({
                    ajax: {
                        url: ajaxurl, dataType: 'json', delay: 250,
                        data: function(params) {
                            return { term: params.term, action: 'woocommerce_json_search_products_and_variations', security: '<?php echo esc_js($nonce); ?>' };
                        },
                        processResults: function(data) {
                            var t = [];
                            if (data) $.each(data, function(id, text) { t.push({ id: id, text: text }); });
                            return { results: t };
                        }, cache: true
                    }, minimumInputLength: 1
                });
            }
            $sel.on('change', function() {
                var vals = $(this).val() || [];
                if (vals.length > MAX) { alert('最多 ' + MAX + ' 个'); vals.pop(); $(this).val(vals).trigger('change.select2'); }
                $('#osb-count-num').text(($(this).val() || []).length);
            });
            $('#osb-reset-index').on('click', function(e) {
                e.preventDefault();
                if (!confirm('确定重置轮询指针到 0？')) return;
                $.post(ajaxurl, { action: 'osb_reset_poll_index', nonce: '<?php echo esc_js($reset_nonce); ?>' }, function(res) {
                    if (res.success) { $('#osb-poll-index').text('0'); alert('已重置'); }
                });
            });
        });
        </script>
        <?php
    }

    public function process_payment($order_id): array { return ['result' => 'fail']; }
    public function get_setting(string $key, string $default = ''): string { return $this->get_option($key, $default); }
}
