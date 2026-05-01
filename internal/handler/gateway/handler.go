package gateway

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"nmegateway/internal/model"
	hmacutil "nmegateway/internal/pkg/hmac"
	"nmegateway/internal/pkg/response"
	"nmegateway/internal/service"
)

type Handler struct {
	Gateway     *service.GatewayService
	HMACWindowS int64
}

type orderReq struct {
	AOrderID      string  `json:"a_order_id"`
	Amount        float64 `json:"amount"`
	PaymentMethod string  `json:"payment_method"`
	Currency      string  `json:"currency"`
	Email         string  `json:"email"`
	IP            string  `json:"ip"`
	ReturnURL     string  `json:"return_url"`
	CheckoutURL   string  `json:"checkout_url"`
	ValidityDays  *int    `json:"validity_days"` // nil=全局默认, 0=仅210s, 1/7/30/90/180
}

// Order — A站调SS下单，用 api_keys 表验证
func (h *Handler) Order(c *gin.Context) {
	apiKey := c.GetHeader("X-Api-Key")
	ts     := c.GetHeader("X-Timestamp")
	sig    := c.GetHeader("X-Signature")
	if apiKey == "" || ts == "" || sig == "" {
		response.Fail(c, http.StatusUnauthorized, "missing signature headers")
		return
	}
	unixTs, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		response.Fail(c, http.StatusUnauthorized, "invalid timestamp")
		return
	}
	rawBody, err := io.ReadAll(c.Request.Body)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "cannot read body")
		return
	}
	_, secret, user, authErr := h.Gateway.ResolveUserByAPIKey(apiKey)
	if authErr != nil {
		response.Fail(c, http.StatusUnauthorized, authErr.Error())
		return
	}
	if !hmacutil.VerifyBodyRequest(apiKey, unixTs, sig, rawBody, secret, h.HMACWindowS) {
		response.Fail(c, http.StatusUnauthorized, "invalid signature")
		return
	}
	var req orderReq
	if err := json.Unmarshal(rawBody, &req); err != nil || req.AOrderID == "" || req.Amount <= 0 {
		response.Fail(c, http.StatusBadRequest, "invalid payload")
		return
	}
	if req.PaymentMethod != "stripe" && req.PaymentMethod != "paypal" {
		req.PaymentMethod = "stripe"
	}
	if req.Currency == "" {
		req.Currency = "USD"
	}
	ip := req.IP
	if ip == "" {
		ip = c.ClientIP()
	}
	score, blocked := func() (int, bool) {
		result := service.AssessRiskFull(h.Gateway.DB, h.Gateway.RDB, req.Amount, ip, user.ID)
		if result.Blocked && h.Gateway.AlertSvc != nil {
			h.Gateway.AlertSvc.HighRiskTransaction(0, req.AOrderID, result.Score, result.Reasons)
		}
		return result.Score, result.Blocked
	}()
	if blocked {
		response.Fail(c, http.StatusForbidden, "risk blocked")
		return
	}
	order, err := h.Gateway.CreateOrder(user, service.CreateOrderParams{
		AOrderID:      req.AOrderID,
		Amount:        req.Amount,
		PaymentMethod: req.PaymentMethod,
		Currency:      req.Currency,
		Email:         req.Email,
		IP:            ip,
		ReturnURL:     req.ReturnURL,
		CheckoutURL:   req.CheckoutURL,
		RiskScore:     score,
		ValidityDays:  req.ValidityDays,
	})
	if err != nil {
		response.Fail(c, http.StatusBadGateway, err.Error())
		return
	}
	// 下单成功后才递增风控计数（与风控检查解耦，避免误伤）
	go service.IncrIPCount(h.Gateway.RDB, ip)
	go service.IncrUserDailyCount(h.Gateway.RDB, user.ID)
	response.OK(c, gin.H{
		"risk_score":     score,
		"payment_url":    order.PaymentURL,
		"pay_token":      order.PayToken,
		"order_status":   order.Status,
		"payment_method": order.PaymentMethod,
		"expires_at":     order.ExpiresAt,
		"validity_days":  order.ValidityDays,
	})
}

// Callback — B站回调SS通知支付结果，用 webhook_endpoints 的 b_api_key 验证
func (h *Handler) Callback(c *gin.Context) {
	bApiKey := c.GetHeader("X-Api-Key")
	ts      := c.GetHeader("X-Timestamp")
	sig     := c.GetHeader("X-Signature")
	if bApiKey == "" || ts == "" || sig == "" {
		response.Fail(c, http.StatusUnauthorized, "missing signature headers")
		return
	}
	unixTs, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		response.Fail(c, http.StatusUnauthorized, "invalid timestamp")
		return
	}
	rawBody, err := io.ReadAll(c.Request.Body)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "cannot read body")
		return
	}

	// 用 B站专属密钥验证（webhook_endpoints 表，type=b）
	bSecret, authErr := h.Gateway.ResolveByBApiKey(bApiKey)
	if authErr != nil {
		response.Fail(c, http.StatusUnauthorized, authErr.Error())
		return
	}
	if !hmacutil.VerifyBodyRequest(bApiKey, unixTs, sig, rawBody, bSecret, h.HMACWindowS) {
		response.Fail(c, http.StatusUnauthorized, "invalid signature")
		return
	}

	var req struct {
		PayToken string  `json:"pay_token"`
		Status   string  `json:"status"`
		Amount   float64 `json:"amount"`
		BOrderID string  `json:"b_order_id"`
		TxID     string  `json:"transaction_id"`
	}
	if err := json.Unmarshal(rawBody, &req); err != nil || req.PayToken == "" {
		response.Fail(c, http.StatusBadRequest, "invalid payload")
		return
	}
	switch req.Status {
	case "completed":
		order, err := h.Gateway.CompleteByPayToken(req.PayToken, req.Amount, req.BOrderID, req.TxID)
		if err != nil {
			response.Fail(c, http.StatusBadRequest, err.Error())
			return
		}
		response.OK(c, gin.H{"order_id": order.ID, "status": order.Status})
	case "failed", "abandoned":
		order, err := h.Gateway.FailByPayToken(req.PayToken, req.Status)
		if err != nil {
			response.Fail(c, http.StatusBadRequest, err.Error())
			return
		}
		response.OK(c, gin.H{"order_id": order.ID, "status": order.Status})
	default:
		response.Fail(c, http.StatusBadRequest, "unsupported status")
	}
}

// Status — 前端轮询订单状态
func (h *Handler) Status(c *gin.Context) {
	var order model.Order
	if err := h.Gateway.DB.Where("pay_token = ?", c.Param("token")).First(&order).Error; err != nil {
		response.Fail(c, http.StatusNotFound, "order not found")
		return
	}
	response.OK(c, gin.H{
		"pay_token":      order.PayToken,
		"a_order_id":     order.AOrderID,
		"status":         order.Status,
		"amount":         order.Amount,
		"payment_method": order.PaymentMethod,
		"payment_url":    order.PaymentURL,
		"return_url":     order.ReturnURL,
		"checkout_url":   order.CheckoutURL,
		"callback_state": order.CallbackState,
	})
}

// GeneratePayLink — B站调NME生成支付链接（新接口）
func (h *Handler) GeneratePayLink(c *gin.Context) {
	bApiKey := c.GetHeader("X-Api-Key")
	ts      := c.GetHeader("X-Timestamp")
	sig     := c.GetHeader("X-Signature")
	if bApiKey == "" || ts == "" || sig == "" {
		response.Fail(c, http.StatusUnauthorized, "missing signature headers")
		return
	}
	unixTs, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		response.Fail(c, http.StatusUnauthorized, "invalid timestamp")
		return
	}
	rawBody, err := io.ReadAll(c.Request.Body)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "cannot read body")
		return
	}
	bSecret, authErr := h.Gateway.ResolveByBApiKey(bApiKey)
	if authErr != nil {
		response.Fail(c, http.StatusUnauthorized, authErr.Error())
		return
	}
	if !hmacutil.VerifyBodyRequest(bApiKey, unixTs, sig, rawBody, bSecret, h.HMACWindowS) {
		response.Fail(c, http.StatusUnauthorized, "invalid signature")
		return
	}

	var req struct {
		PayToken      string  `json:"pay_token"`
		Amount        float64 `json:"amount"`
		Currency      string  `json:"currency"`
		PaymentMethod string  `json:"payment_method"`
		Email         string  `json:"email"`
		SuccessURL    string  `json:"success_url"`
		CancelURL     string  `json:"cancel_url"`
		Description   string  `json:"description"`
	}
	if err := json.Unmarshal(rawBody, &req); err != nil || req.PayToken == "" || req.Amount <= 0 {
		response.Fail(c, http.StatusBadRequest, "invalid payload: pay_token and amount required")
		return
	}

	payLink, err := h.Gateway.GeneratePayLink(bApiKey, req.PayToken, req.Amount, req.Currency, req.PaymentMethod, req.Email, req.SuccessURL, req.CancelURL, req.Description)
	if err != nil {
		response.Fail(c, http.StatusBadGateway, err.Error())
		return
	}
	response.OK(c, gin.H{
		"payment_url":    payLink,
		"pay_token":      req.PayToken,
	})
}

// CircuitBreak 手动熔断一个支付账号（仅 Admin 或内部调用）
func (h *Handler) CircuitBreak(c *gin.Context) {
	var req struct {
		ChannelType string `json:"channel_type"` // paypal|stripe
		ChannelID   uint   `json:"channel_id"`
		Reason      string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ChannelID == 0 {
		response.Fail(c, http.StatusBadRequest, "channel_type and channel_id required")
		return
	}
	if err := h.Gateway.CircuitBreakAccount(req.ChannelType, req.ChannelID, req.Reason); err != nil {
		response.Fail(c, http.StatusBadRequest, err.Error())
		return
	}
	response.OK(c, gin.H{"channel_id": req.ChannelID, "channel_type": req.ChannelType, "isolated": true})
}

// ReportFail 报告支付账号失败次数（B站或外部回报）
func (h *Handler) ReportFail(c *gin.Context) {
	var req struct {
		ChannelType string `json:"channel_type"` // paypal|stripe
		ChannelID   uint   `json:"channel_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ChannelID == 0 {
		response.Fail(c, http.StatusBadRequest, "channel_type and channel_id required")
		return
	}
	h.Gateway.ReportFail(req.ChannelType, req.ChannelID)
	response.OK(c, gin.H{"channel_id": req.ChannelID, "reported": true})
}
