package service

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"nme-v9/internal/model"
	hmacutil "nme-v9/internal/pkg/hmac"
	"nme-v9/internal/task"
)

type GatewayService struct {
	DB       *gorm.DB
	Queue    *asynq.Client
	Log      *zap.Logger
	RDB      *redis.Client
	AlertSvc *AlertService
	SmartSvc *SmartRoutingService // 智能路由服务，用于 RecordRequest 和动态权重
}

// ResolveUserByAPIKey — A站下单用，查 api_keys 表
func (s *GatewayService) ResolveUserByAPIKey(apiKey string) (uint, string, *model.User, error) {
	var key model.APIKey
	if err := s.DB.Where("api_key = ? AND enabled = true", apiKey).First(&key).Error; err != nil {
		return 0, "", nil, errors.New("invalid api key")
	}
	var user model.User
	if err := s.DB.First(&user, key.UserID).Error; err != nil {
		return 0, "", nil, errors.New("user not found")
	}
	if user.Status != "active" {
		return 0, "", nil, errors.New("user inactive")
	}
	if !user.ExpiresAt.IsZero() && user.ExpiresAt.Before(time.Now().UTC()) {
		return 0, "", nil, errors.New("user expired")
	}
	return user.ID, key.Secret, &user, nil
}

// ResolveByBApiKey — B站回调/调pay-link用，查 webhook_endpoints 表 type=b
func (s *GatewayService) ResolveByBApiKey(bApiKey string) (string, error) {
	var ep model.WebhookEndpoint
	if err := s.DB.Where("b_api_key = ? AND type = 'b' AND enabled = true", bApiKey).First(&ep).Error; err != nil {
		return "", errors.New("invalid b_api_key")
	}
	return ep.BSharedSecret, nil
}

// ── Order creation ─────────────────────────────────────────────────────────────

type CreateOrderParams struct {
	AOrderID      string
	Amount        float64
	PaymentMethod string
	Currency      string
	Email         string
	IP            string
	ReturnURL     string
	CheckoutURL   string
	RiskScore     int
	ValidityDays  *int // nil=用全局默认, 0=仅210s短单, 1/7/30/90/180=指定天数
}

func (s *GatewayService) CreateOrder(user *model.User, p CreateOrderParams) (*model.Order, error) {
	// 幂等检查：status=failed 的允许重试
	var exists model.Order
	if err := s.DB.Where("user_id = ? AND a_order_id = ? AND status != 'failed'", user.ID, p.AOrderID).First(&exists).Error; err == nil {
		return &exists, nil
	}

	currency := p.Currency
	if currency == "" {
		currency = "USD"
	}
	paymentMethod := p.PaymentMethod
	if paymentMethod == "" {
		paymentMethod = "stripe"
	}

	// 选支付账号（只从该用户自己的账号池）
	var paypalAccountID *uint
	var stripeConfigID *uint
	var gatewayLabel string

	switch paymentMethod {
	case "paypal":
		acc, err := SelectPaypalAccount(s.DB, s.RDB, s.Log, s.SmartSvc, user.ID, p.Amount)
		if err != nil {
			if s.AlertSvc != nil {
				s.AlertSvc.NoChannel(user.ID, "paypal")
			}
			return nil, fmt.Errorf("no available paypal account: %w", err)
		}
		paypalAccountID = &acc.ID
		gatewayLabel = acc.Label
	default: // stripe
		cfg, err := SelectStripeConfig(s.DB, s.RDB, s.Log, s.SmartSvc, user.ID, p.Amount)
		if err != nil {
			if s.AlertSvc != nil {
				s.AlertSvc.NoChannel(user.ID, "stripe")
			}
			return nil, fmt.Errorf("no available stripe config: %w", err)
		}
		stripeConfigID = &cfg.ID
		gatewayLabel = cfg.Label
	}

	// 计算订单时效
	validityDays := p.ValidityDays
	if validityDays == nil {
		// 读全局默认
		var setting model.GlobalSetting
		if s.DB.Where("key = 'default_validity_days'").First(&setting).Error == nil {
			d := 0
			fmt.Sscanf(setting.Value, "%d", &d)
			if d > 0 {
				validityDays = &d
			}
		}
	}
	var expiresAt *time.Time
	if validityDays != nil && *validityDays > 0 {
		t := time.Now().UTC().AddDate(0, 0, *validityDays)
		expiresAt = &t
	}

	payToken, err := randomToken()
	if err != nil {
		return nil, err
	}

	order := &model.Order{
		UserID:          user.ID,
		AOrderID:        p.AOrderID,
		Amount:          p.Amount,
		Status:          "pending",
		PayToken:        payToken,
		PaymentMethod:   paymentMethod,
		Currency:        currency,
		Email:           p.Email,
		IP:              p.IP,
		ReturnURL:       p.ReturnURL,
		CheckoutURL:     p.CheckoutURL,
		RiskScore:       p.RiskScore,
		CallbackState:   "pending",
		PaypalAccountID: paypalAccountID,
		StripeConfigID:  stripeConfigID,
		GatewayLabel:    gatewayLabel,
		ValidityDays:    validityDays,
		ExpiresAt:       expiresAt,
	}
	if err := s.DB.Create(order).Error; err != nil {
		return nil, err
	}

	bResult, err := s.CallBStation(user, order)
	if err != nil || !bResult.Success {
		errMsg := "B-station call failed"
		if err != nil {
			errMsg = err.Error()
		} else if bResult.Error != "" {
			errMsg = bResult.Error
		}
		s.DB.Model(order).Update("status", "failed")
		s.log().Warn("B站建单失败", zap.String("a_order_id", p.AOrderID), zap.String("error", errMsg))
		// B站失败时记录 SmartRouting 失败指标
		if s.SmartSvc != nil {
			if paypalAccountID != nil {
				go s.SmartSvc.RecordRequest("paypal", int64(*paypalAccountID), gatewayLabel, false, 0, true)
			} else if stripeConfigID != nil {
				go s.SmartSvc.RecordRequest("stripe", int64(*stripeConfigID), gatewayLabel, false, 0, true)
			}
		}
		return nil, errors.New(errMsg)
	}

	s.DB.Model(order).Updates(map[string]any{
		"b_order_id":  bResult.BOrderID,
		"payment_url": bResult.PaymentURL,
	})
	order.BOrderID = bResult.BOrderID
	order.PaymentURL = bResult.PaymentURL

	if s.Queue != nil {
		if t, e := task.NewCheckAbandonedTask(order.ID); e == nil {
			_, _ = s.Queue.Enqueue(t, asynq.ProcessIn(210*time.Second), asynq.MaxRetry(1))
		}
	}

	s.log().Info("Gateway order created",
		zap.String("a_order_id", p.AOrderID),
		zap.String("payment_method", paymentMethod),
		zap.String("gateway_label", gatewayLabel),
		zap.Any("validity_days", validityDays),
	)
	return order, nil
}

// ── B站 outbound call ─────────────────────────────────────────────────────────

type BStationResult struct {
	Success    bool
	PaymentURL string
	BOrderID   string
	Error      string
}

func (s *GatewayService) CallBStation(user *model.User, order *model.Order) (*BStationResult, error) {
	var endpoint model.WebhookEndpoint
	err := s.DB.Where("user_id = ? AND type = 'b' AND enabled = true", user.ID).
		Order("id desc").First(&endpoint).Error
	if err != nil {
		return &BStationResult{Success: false, Error: "no B-station endpoint configured"}, nil
	}
	if endpoint.BApiKey == "" || endpoint.BSharedSecret == "" {
		return &BStationResult{Success: false, Error: "B-station endpoint missing credentials"}, nil
	}

	baseURL := strings.TrimRight(strings.Split(endpoint.URL, "/wp-json")[0], "/")
	bURL := baseURL + "/wp-json/b-station/v1/order"

	payload := map[string]any{
		"a_order_id":     order.AOrderID,
		"pay_token":      order.PayToken,
		"email":          order.Email,
		"ip":             order.IP,
		"amount":         fmt.Sprintf("%.2f", order.Amount),
		"currency":       order.Currency,
		"payment_method": order.PaymentMethod,
	}
	body, _ := json.Marshal(payload)
	headers := hmacutil.BuildHeaders(endpoint.BApiKey, endpoint.BSharedSecret, body)

	req, _ := http.NewRequest("POST", bURL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return &BStationResult{Success: false, Error: err.Error()}, nil
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &BStationResult{Success: false, Error: fmt.Sprintf("B-station HTTP %d", resp.StatusCode)}, nil
	}

	var data map[string]any
	if err := json.Unmarshal(respBody, &data); err != nil {
		return &BStationResult{Success: false, Error: "invalid B-station response"}, nil
	}

	paymentURL, _ := data["payment_url"].(string)
	bOrderID, _ := data["b_order_id"].(string)
	if bOrderID == "" {
		bOrderID, _ = data["order_id"].(string)
	}

	return &BStationResult{
		Success:    true,
		PaymentURL: paymentURL,
		BOrderID:   bOrderID,
	}, nil
}

// ── GeneratePayLink — B站调NME生成支付链接 ────────────────────────────────────
// 通过 b_api_key 找到对应的 webhook_endpoint → 找到所属用户 → 从该用户自己的账号池选账号

func (s *GatewayService) GeneratePayLink(bApiKey string, payToken string, amount float64, currency, paymentMethod, email, successURL, cancelURL, description string) (string, error) {
	if currency == "" {
		currency = "USD"
	}

	// 通过 b_api_key 找 webhook_endpoint，再找用户
	var ep model.WebhookEndpoint
	if err := s.DB.Where("b_api_key = ? AND type = 'b' AND enabled = true", bApiKey).First(&ep).Error; err != nil {
		return "", errors.New("invalid b_api_key, cannot resolve user")
	}
	userID := ep.UserID

	switch paymentMethod {
	case "stripe":
		// 从该用户的 stripe_configs 选一个可用账号
		cfg, err := SelectStripeConfig(s.DB, s.RDB, s.Log, s.SmartSvc, userID, amount)
		if err != nil {
			return "", fmt.Errorf("no available stripe config: %w", err)
		}
		secretKey := cfg.SecretKey
		if cfg.Sandbox {
			secretKey = cfg.SecretKey // sandbox 用 sandbox key（用户自行配置）
		}
		return s.createStripeSession(secretKey, amount, currency, email, successURL, cancelURL, description, payToken)

	case "paypal":
		// 从该用户的 paypal_accounts 选一个可用账号
		acc, err := SelectPaypalAccount(s.DB, s.RDB, s.Log, s.SmartSvc, userID, amount)
		if err != nil {
			return "", fmt.Errorf("no available paypal account: %w", err)
		}
		// email 模式：直接构造 PayPal.me 链接
		if acc.Mode == "email" {
			activeEmail := acc.ActiveEmail()
			if activeEmail == "" {
				return "", errors.New("paypal account email is empty")
			}
			// PayPal.me 链接格式：https://paypal.me/USERNAME/AMOUNT
			// 仅在 email 模式下退化为 email 前缀，避免错误使用 TrimPrefix("", "")。
			username := strings.Split(activeEmail, "@")[0]
			paypalURL := fmt.Sprintf("https://paypal.me/%s/%.2f%s",
				username, amount, strings.ToUpper(currency))
			return paypalURL, nil
		}
		// rest 模式：走 OAuth2 API
		clientID := acc.ClientID
		secret := acc.ClientSecret
		if acc.SandboxMode {
			clientID = acc.SandboxClientID
			secret = acc.SandboxClientSecret
		}
		return s.createPayPalOrder(clientID, secret, amount, currency, successURL, cancelURL, description)

	default:
		return "", errors.New("unsupported payment_method: use stripe or paypal")
	}
}

func (s *GatewayService) createStripeSession(secretKey string, amount float64, currency, email, successURL, cancelURL, description, payToken string) (string, error) {
	amountCents := int64(amount * 100)
	formData := fmt.Sprintf(
		"line_items[0][price_data][currency]=%s&line_items[0][price_data][unit_amount]=%d&line_items[0][price_data][product_data][name]=%s&line_items[0][quantity]=1&mode=payment&success_url=%s&cancel_url=%s&client_reference_id=%s",
		currency, amountCents, urlEncode(description), urlEncode(successURL), urlEncode(cancelURL), urlEncode(payToken),
	)
	if email != "" {
		formData += "&customer_email=" + urlEncode(email)
	}

	req, _ := http.NewRequest("POST", "https://api.stripe.com/v1/checkout/sessions", strings.NewReader(formData))
	req.Header.Set("Authorization", "Bearer "+secretKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("stripe request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return "", errors.New("invalid stripe response")
	}
	if resp.StatusCode != 200 {
		errMsg, _ := result["error"].(map[string]any)
		msg := "stripe error"
		if errMsg != nil {
			msg, _ = errMsg["message"].(string)
		}
		return "", errors.New(msg)
	}
	url, _ := result["url"].(string)
	if url == "" {
		return "", errors.New("stripe returned no payment URL")
	}
	return url, nil
}

func (s *GatewayService) createPayPalOrder(clientID, secret string, amount float64, currency, successURL, cancelURL, description string) (string, error) {
	// Step 1: 获取 access token
	tokenReq, _ := http.NewRequest("POST", "https://api-m.paypal.com/v1/oauth2/token",
		strings.NewReader("grant_type=client_credentials"))
	tokenReq.SetBasicAuth(clientID, secret)
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 15 * time.Second}
	tokenResp, err := client.Do(tokenReq)
	if err != nil {
		return "", fmt.Errorf("paypal token request failed: %w", err)
	}
	defer tokenResp.Body.Close()
	tokenBody, _ := io.ReadAll(tokenResp.Body)

	var tokenData map[string]any
	json.Unmarshal(tokenBody, &tokenData)
	accessToken, _ := tokenData["access_token"].(string)
	if accessToken == "" {
		return "", errors.New("failed to get PayPal access token")
	}

	// Step 2: 创建 order
	orderPayload := map[string]any{
		"intent": "CAPTURE",
		"purchase_units": []map[string]any{{
			"amount":      map[string]any{"currency_code": currency, "value": fmt.Sprintf("%.2f", amount)},
			"description": description,
		}},
		"application_context": map[string]any{
			"return_url": successURL,
			"cancel_url": cancelURL,
		},
	}
	orderBody, _ := json.Marshal(orderPayload)
	orderReq, _ := http.NewRequest("POST", "https://api-m.paypal.com/v2/checkout/orders", bytes.NewReader(orderBody))
	orderReq.Header.Set("Authorization", "Bearer "+accessToken)
	orderReq.Header.Set("Content-Type", "application/json")

	orderResp, err := client.Do(orderReq)
	if err != nil {
		return "", fmt.Errorf("paypal order request failed: %w", err)
	}
	defer orderResp.Body.Close()
	orderRespBody, _ := io.ReadAll(orderResp.Body)

	var orderData map[string]any
	json.Unmarshal(orderRespBody, &orderData)

	links, _ := orderData["links"].([]any)
	for _, l := range links {
		link, _ := l.(map[string]any)
		if rel, _ := link["rel"].(string); rel == "approve" {
			if href, _ := link["href"].(string); href != "" {
				return href, nil
			}
		}
	}
	return "", errors.New("PayPal did not return an approval URL")
}

func urlEncode(s string) string {
	return url.QueryEscape(s)
}

// ── Order completion ───────────────────────────────────────────────────────────

func (s *GatewayService) CompleteByPayToken(token string, reportAmount float64, bOrderID, bTxID string) (*model.Order, error) {
	var order model.Order
	if err := s.DB.Where("pay_token = ?", token).First(&order).Error; err != nil {
		return nil, errors.New("order not found")
	}
	if order.Status == "completed" {
		return &order, nil
	}
	if order.Status != "pending" {
		return nil, errors.New("order status not allowed")
	}
	if reportAmount > 0 {
		diff := reportAmount - order.Amount
		if diff < 0 {
			diff = -diff
		}
		if diff > 0.02 {
			s.log().Warn("回调金额不符",
				zap.String("a_order_id", order.AOrderID),
				zap.Float64("expected", order.Amount),
				zap.Float64("got", reportAmount),
			)
		}
	}
	now := time.Now()
	updates := map[string]any{"status": "completed", "paid_at": &now}
	if bOrderID != "" {
		updates["b_order_id"] = bOrderID
	}
	if bTxID != "" {
		updates["b_transaction_id"] = bTxID
	}
	if err := s.DB.Model(&order).Updates(updates).Error; err != nil {
		return nil, err
	}
	order.Status = "completed"
	order.PaidAt = &now

	if s.Queue != nil {
		if t, e := task.NewCallbackTask(order.ID, "completed"); e == nil {
			_, _ = s.Queue.Enqueue(t, asynq.MaxRetry(3))
		}
	}
	// 同步通知 B站 complete（告知支付已完成）
	go s.CallbackBStation(&order)
	return &order, nil
}

func (s *GatewayService) FailByPayToken(token string, status string) (*model.Order, error) {
	var order model.Order
	if err := s.DB.Where("pay_token = ?", token).First(&order).Error; err != nil {
		return nil, errors.New("order not found")
	}
	if order.Status == "failed" || order.Status == "abandoned" {
		return &order, nil
	}
	if order.Status != "pending" {
		return nil, errors.New("order status not allowed")
	}
	if status != "abandoned" {
		status = "failed"
	}
	order.Status = status
	if err := s.DB.Save(&order).Error; err != nil {
		return nil, err
	}
	if s.Queue != nil {
		if t, e := task.NewCallbackTask(order.ID, status); e == nil {
			_, _ = s.Queue.Enqueue(t, asynq.MaxRetry(3))
		}
	}
	return &order, nil
}

// ── A站回调 ────────────────────────────────────────────────────────────────────

func (s *GatewayService) CallbackAStation(order *model.Order) {
	var user model.User
	if err := s.DB.First(&user, order.UserID).Error; err != nil {
		return
	}
	var endpoint model.WebhookEndpoint
	err := s.DB.Where("user_id = ? AND type = 'a' AND enabled = true AND (payment_method = 'all' OR payment_method = ?)",
		user.ID, order.PaymentMethod).First(&endpoint).Error
	if err != nil {
		s.log().Warn("无A站端点配置", zap.Uint("order_id", order.ID))
		return
	}
	if endpoint.URL == "" || endpoint.AApiKey == "" || endpoint.SharedSecret == "" {
		s.log().Warn("A站端点配置不完整", zap.Uint("endpoint_id", endpoint.ID))
		return
	}

	// 修复：status 根据实际订单状态传递
	statusVal := order.Status
	if statusVal == "completed" {
		statusVal = "paid"
	}

	payload := map[string]any{
		"order_id":       order.AOrderID,
		"b_order_id":     order.BOrderID,
		"amount":         fmt.Sprintf("%.2f", order.Amount),
		"status":         statusVal,
		"nme_order_id":   order.ID,
		"transaction_id": order.BTransactionID,
	}
	body, _ := json.Marshal(payload)
	headers := hmacutil.BuildHeaders(endpoint.AApiKey, endpoint.SharedSecret, body)

	req, _ := http.NewRequest("POST", endpoint.URL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		s.log().Error("A站回调失败", zap.Uint("order_id", order.ID), zap.Error(err))
		return
	}
	defer resp.Body.Close()
	s.log().Info("A站回调完成", zap.Uint("order_id", order.ID), zap.Int("http", resp.StatusCode))
	s.DB.Model(order).Update("callback_state", "done")
}

// CallbackBStation 支付完成后主动通知 B站更新订单状态
func (s *GatewayService) CallbackBStation(order *model.Order) {
	var user model.User
	if err := s.DB.First(&user, order.UserID).Error; err != nil {
		return
	}
	var endpoint model.WebhookEndpoint
	err := s.DB.Where("user_id = ? AND type = 'b' AND enabled = true", user.ID).
		Order("id desc").First(&endpoint).Error
	if err != nil || endpoint.BApiKey == "" || endpoint.BSharedSecret == "" {
		return
	}

	baseURL := strings.TrimRight(strings.Split(endpoint.URL, "/wp-json")[0], "/")
	bURL := baseURL + "/wp-json/b-station/v1/complete"

	payload := map[string]any{
		"pay_token":      order.PayToken,
		"a_order_id":     order.AOrderID,
		"b_order_id":     order.BOrderID,
		"status":         order.Status,
		"amount":         fmt.Sprintf("%.2f", order.Amount),
		"transaction_id": order.BTransactionID,
	}
	body, _ := json.Marshal(payload)
	headers := hmacutil.BuildHeaders(endpoint.BApiKey, endpoint.BSharedSecret, body)

	req, _ := http.NewRequest("POST", bURL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		s.log().Warn("B站 complete 回调失败", zap.Uint("order_id", order.ID), zap.Error(err))
		return
	}
	defer resp.Body.Close()
	s.log().Info("B站 complete 回调完成", zap.Uint("order_id", order.ID), zap.Int("http", resp.StatusCode))
}

// CircuitBreakAccount 手动熔断一个支付账号
func (s *GatewayService) CircuitBreakAccount(channelType string, channelID uint, reason string) error {
	switch channelType {
	case "paypal":
		if err := s.DB.Model(&model.PaypalAccount{}).Where("id = ?", channelID).Updates(map[string]any{
			"account_state": "abandoned",
			"enabled":       false,
		}).Error; err != nil {
			return err
		}
	case "stripe":
		if err := s.DB.Model(&model.StripeConfig{}).Where("id = ?", channelID).Updates(map[string]any{
			"account_state": "abandoned",
			"enabled":       false,
		}).Error; err != nil {
			return err
		}
	default:
		return errors.New("channel_type must be paypal or stripe")
	}
	if s.AlertSvc != nil {
		s.AlertSvc.ChannelIsolated(channelType, channelID, fmt.Sprintf("id:%d", channelID), reason)
	}
	s.log().Warn("Circuit break triggered",
		zap.String("channel_type", channelType),
		zap.Uint("channel_id", channelID),
		zap.String("reason", reason),
	)
	return nil
}

// ReportFail 报告账号支付失败，fail_count 达到阈值后自动熔断
func (s *GatewayService) ReportFail(channelType string, channelID uint) {
	var threshold int
	var setting model.GlobalSetting
	if err := s.DB.Where("key = 'chargeback_threshold'").First(&setting).Error; err == nil {
		fmt.Sscanf(setting.Value, "%d", &threshold)
	}
	if threshold <= 0 {
		threshold = 3
	}

	switch channelType {
	case "paypal":
		var acc model.PaypalAccount
		if err := s.DB.First(&acc, channelID).Error; err != nil {
			return
		}
		s.DB.Model(&acc).UpdateColumn("fail_count", gorm.Expr("fail_count + 1"))
		acc.FailCount++
		if acc.FailCount >= threshold {
			s.DB.Model(&acc).Updates(map[string]any{"account_state": "abandoned", "enabled": false})
			if s.AlertSvc != nil {
				s.AlertSvc.ChargebackThreshold("paypal", channelID, acc.Label, acc.FailCount)
			}
		}
	case "stripe":
		var cfg model.StripeConfig
		if err := s.DB.First(&cfg, channelID).Error; err != nil {
			return
		}
		s.DB.Model(&cfg).UpdateColumn("fail_count", gorm.Expr("fail_count + 1"))
		cfg.FailCount++
		if cfg.FailCount >= threshold {
			s.DB.Model(&cfg).Updates(map[string]any{"account_state": "abandoned", "enabled": false})
			if s.AlertSvc != nil {
				s.AlertSvc.ChargebackThreshold("stripe", channelID, cfg.Label, cfg.FailCount)
			}
		}
	}
}

func randomToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *GatewayService) log() *zap.Logger {
	if s.Log != nil {
		return s.Log
	}
	return zap.NewNop()
}
