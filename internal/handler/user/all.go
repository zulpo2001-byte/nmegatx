package user

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"nme-v9/internal/model"
	"nme-v9/internal/pkg/response"
)

type Handler struct {
	DB      *gorm.DB
	BaseURL string // e.g. https://nme.example.com
}

// ════════════════════════════════════════════════════════
// 产品通道管理
// ════════════════════════════════════════════════════════

func (h *Handler) Products(c *gin.Context) {
	userID := c.GetUint("user_id")
	var items []model.Product
	if err := h.DB.Where("user_id = ?", userID).Order("poll_order asc, id asc").Find(&items).Error; err != nil {
		response.Fail(c, http.StatusInternalServerError, "query products failed")
		return
	}
	response.OK(c, gin.H{"items": items})
}

func (h *Handler) CreateProduct(c *gin.Context) {
	userID := c.GetUint("user_id")
	var req struct {
		Label      string `json:"label"`
		BProductID string `json:"b_product_id"`
		Weight     int    `json:"weight"`
		PollOrder  *int   `json:"poll_order"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.BProductID == "" {
		response.Fail(c, http.StatusBadRequest, "invalid payload: b_product_id required")
		return
	}
	if req.Weight <= 0 {
		req.Weight = 1
	}
	// Auto poll_order = max + 1
	var maxOrder int
	h.DB.Model(&model.Product{}).Where("user_id = ?", userID).Select("COALESCE(MAX(poll_order), -1)").Scan(&maxOrder)
	pollOrder := maxOrder + 1
	if req.PollOrder != nil && *req.PollOrder >= 0 {
		pollOrder = *req.PollOrder
	}

	p := model.Product{
		UserID:     userID,
		Label:      req.Label,
		BProductID: req.BProductID,
		Weight:     req.Weight,
		PollOrder:  pollOrder,
		Enabled:    true,
	}
	if err := h.DB.Create(&p).Error; err != nil {
		response.Fail(c, http.StatusBadRequest, "create product failed")
		return
	}
	response.OK(c, p)
}

func (h *Handler) UpdateProduct(c *gin.Context) {
	userID := c.GetUint("user_id")
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req map[string]any
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid payload")
		return
	}
	delete(req, "id")
	delete(req, "user_id")
	delete(req, "total_used")
	delete(req, "last_used_at")
	if err := h.DB.Model(&model.Product{}).Where("id = ? AND user_id = ?", id, userID).Updates(req).Error; err != nil {
		response.Fail(c, http.StatusInternalServerError, "update product failed")
		return
	}
	response.OK(c, gin.H{"id": id, "updated": true})
}

func (h *Handler) DeleteProduct(c *gin.Context) {
	userID := c.GetUint("user_id")
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := h.DB.Where("id = ? AND user_id = ?", id, userID).Delete(&model.Product{}).Error; err != nil {
		response.Fail(c, http.StatusInternalServerError, "delete product failed")
		return
	}
	response.OK(c, gin.H{"id": id, "deleted": true})
}

func (h *Handler) ToggleProduct(c *gin.Context) {
	userID := c.GetUint("user_id")
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var p model.Product
	if err := h.DB.Where("id = ? AND user_id = ?", id, userID).First(&p).Error; err != nil {
		response.Fail(c, http.StatusNotFound, "product not found")
		return
	}
	h.DB.Model(&p).Update("enabled", !p.Enabled)
	response.OK(c, gin.H{"id": p.ID, "enabled": !p.Enabled})
}

func (h *Handler) ReorderProducts(c *gin.Context) {
	userID := c.GetUint("user_id")
	var req struct {
		Items []struct {
			ID        uint `json:"id"`
			PollOrder int  `json:"poll_order"`
		} `json:"items"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid payload")
		return
	}
	for _, item := range req.Items {
		h.DB.Model(&model.Product{}).Where("id = ? AND user_id = ?", item.ID, userID).Update("poll_order", item.PollOrder)
	}
	response.OK(c, gin.H{"updated": len(req.Items)})
}

// ════════════════════════════════════════════════════════
// 负载均衡策略
// ════════════════════════════════════════════════════════

func (h *Handler) Strategy(c *gin.Context) {
	userID := c.GetUint("user_id")
	var user model.User
	if err := h.DB.First(&user, userID).Error; err != nil {
		response.Fail(c, http.StatusInternalServerError, "user not found")
		return
	}
	var products []model.Product
	h.DB.Where("user_id = ?", userID).Order("poll_order asc, id asc").Find(&products)

	response.OK(c, gin.H{
		"strategy": user.ProductStrategy,
		"products": products,
		"strategy_desc": map[string]string{
			"round_robin": "轮询：选最久未被使用的产品（按 last_used_at），流量均匀分配",
			"random":      "加权随机：按 weight 权重概率选取，weight 越大命中率越高",
			"fixed":       "固定：始终选 poll_order 最小的产品，手动禁用后才切换",
		},
	})
}

func (h *Handler) UpdateStrategy(c *gin.Context) {
	userID := c.GetUint("user_id")
	var req struct {
		Strategy string `json:"strategy"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid payload")
		return
	}
	if req.Strategy != "round_robin" && req.Strategy != "random" && req.Strategy != "fixed" {
		response.Fail(c, http.StatusBadRequest, "strategy must be round_robin|random|fixed")
		return
	}
	if err := h.DB.Model(&model.User{}).Where("id = ?", userID).Update("product_strategy", req.Strategy).Error; err != nil {
		response.Fail(c, http.StatusInternalServerError, "update strategy failed")
		return
	}
	response.OK(c, gin.H{"strategy": req.Strategy})
}

// ════════════════════════════════════════════════════════
// 订单
// ════════════════════════════════════════════════════════

func (h *Handler) Orders(c *gin.Context) {
	userID := c.GetUint("user_id")
	// 支持时间范围过滤: days=1|7|30|90|180，不传=全部
	daysStr := c.Query("days")
	status := c.Query("status")
	query := h.DB.Where("user_id = ?", userID).Order("id desc").Limit(500)
	if daysStr != "" {
		days := 0
		if _, err := fmt.Sscanf(daysStr, "%d", &days); err == nil && days > 0 {
			since := time.Now().UTC().AddDate(0, 0, -days)
			query = query.Where("created_at >= ?", since)
		}
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	var items []model.Order
	if err := query.Find(&items).Error; err != nil {
		response.Fail(c, http.StatusInternalServerError, "query orders failed")
		return
	}
	response.OK(c, gin.H{"items": items})
}

// ════════════════════════════════════════════════════════
// API Keys
// ════════════════════════════════════════════════════════

func (h *Handler) APIKeys(c *gin.Context) {
	userID := c.GetUint("user_id")
	var items []model.APIKey
	if err := h.DB.Where("user_id = ?", userID).Order("id desc").Find(&items).Error; err != nil {
		response.Fail(c, http.StatusInternalServerError, "query api keys failed")
		return
	}
	response.OK(c, gin.H{"items": items})
}

func (h *Handler) CreateAPIKey(c *gin.Context) {
	userID := c.GetUint("user_id")
	key    := "ak_" + randHex(10)
	secret := "sk_" + randHex(20)
	item   := model.APIKey{UserID: userID, APIKey: key, Secret: secret, Enabled: true}
	if err := h.DB.Create(&item).Error; err != nil {
		response.Fail(c, http.StatusBadRequest, "create api key failed")
		return
	}
	response.OK(c, gin.H{"id": item.ID, "api_key": item.APIKey, "secret": secret})
}

func (h *Handler) DeleteAPIKey(c *gin.Context) {
	userID := c.GetUint("user_id")
	id, _  := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := h.DB.Where("id = ? AND user_id = ?", id, userID).Delete(&model.APIKey{}).Error; err != nil {
		response.Fail(c, http.StatusInternalServerError, "delete api key failed")
		return
	}
	response.OK(c, gin.H{"id": id, "deleted": true})
}

// ════════════════════════════════════════════════════════
// Webhook 端点（A站 / B站）
// ════════════════════════════════════════════════════════

func (h *Handler) Webhooks(c *gin.Context) {
	userID := c.GetUint("user_id")
	var items []model.WebhookEndpoint
	if err := h.DB.Where("user_id = ?", userID).Order("id desc").Find(&items).Error; err != nil {
		response.Fail(c, http.StatusInternalServerError, "query webhooks failed")
		return
	}
	// Mask secrets, expose only api_key identifiers
	type safeEP struct {
		ID            uint      `json:"id"`
		Type          string    `json:"type"`
		Label         string    `json:"label"`
		URL           string    `json:"url"`
		PaymentMethod string    `json:"payment_method"`
		Enabled       bool      `json:"enabled"`
		AApiKey       string    `json:"a_api_key,omitempty"`
		BApiKey       string    `json:"b_api_key,omitempty"`
		CreatedAt     time.Time `json:"created_at"`
	}
	var safe []safeEP
	for _, ep := range items {
		s := safeEP{
			ID: ep.ID, Type: ep.Type, Label: ep.Label, URL: ep.URL,
			PaymentMethod: ep.PaymentMethod, Enabled: ep.Enabled, CreatedAt: ep.CreatedAt,
		}
		if ep.Type == "a" {
			s.AApiKey = ep.AApiKey
		} else {
			s.BApiKey = ep.BApiKey
		}
		safe = append(safe, s)
	}
	response.OK(c, gin.H{"items": safe})
}

func (h *Handler) CreateWebhookA(c *gin.Context) {
	userID := c.GetUint("user_id")
	var req struct {
		Label         string `json:"label"`
		URL           string `json:"url"`
		PaymentMethod string `json:"payment_method"` // all|stripe|paypal
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.URL == "" {
		response.Fail(c, http.StatusBadRequest, "url required")
		return
	}
	pm := req.PaymentMethod
	if pm != "all" && pm != "stripe" && pm != "paypal" {
		pm = "all"
	}
	sharedSecret := "whsec_" + randHex(20)
	aApiKey      := "ak_cb_" + randHex(16)

	item := model.WebhookEndpoint{
		UserID:        userID,
		Type:          "a",
		Label:         req.Label,
		URL:           req.URL,
		PaymentMethod: pm,
		Enabled:       true,
		SharedSecret:  sharedSecret,
		AApiKey:       aApiKey,
	}
	if err := h.DB.Create(&item).Error; err != nil {
		response.Fail(c, http.StatusBadRequest, "create webhook failed")
		return
	}
	configString := buildConfigString("a", h.BaseURL, map[string]string{
		"ak": aApiKey, "whsec": sharedSecret,
	})
	response.OK(c, gin.H{
		"id":            item.ID,
		"type":          "a",
		"label":         item.Label,
		"url":           item.URL,
		"payment_method": pm,
		"a_api_key":     aApiKey,
		"shared_secret": sharedSecret,
		"config_string": configString,
		"config_hint":   "将此配置串粘贴到 A站插件「一码配置串」输入框",
		"callback_url":  strings.TrimRight(h.BaseURL, "/") + "/api/gateway/callback",
	})
}

func (h *Handler) CreateWebhookB(c *gin.Context) {
	userID := c.GetUint("user_id")
	var req struct {
		Label         string `json:"label"`
		URL           string `json:"url"`
		PaymentMethod string `json:"payment_method"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.URL == "" {
		response.Fail(c, http.StatusBadRequest, "url required")
		return
	}
	pm := req.PaymentMethod
	if pm != "all" && pm != "stripe" && pm != "paypal" {
		pm = "all"
	}
	bApiKey       := "bk_live_" + randHex(16)
	bSharedSecret := "bsk_" + randHex(20)

	item := model.WebhookEndpoint{
		UserID:        userID,
		Type:          "b",
		Label:         req.Label,
		URL:           req.URL,
		PaymentMethod: pm,
		Enabled:       true,
		BApiKey:       bApiKey,
		BSharedSecret: bSharedSecret,
	}
	if err := h.DB.Create(&item).Error; err != nil {
		response.Fail(c, http.StatusBadRequest, "create webhook failed")
		return
	}
	configString := buildConfigString("b", h.BaseURL, map[string]string{
		"bk": bApiKey, "bsk": bSharedSecret,
	})
	response.OK(c, gin.H{
		"id":              item.ID,
		"type":            "b",
		"label":           item.Label,
		"url":             item.URL,
		"payment_method":  pm,
		"b_api_key":       bApiKey,
		"b_shared_secret": bSharedSecret,
		"config_string":   configString,
		"config_hint":     "将此配置串粘贴到 B站插件「一码配置串」输入框",
	})
}

func (h *Handler) UpdateWebhook(c *gin.Context) {
	userID := c.GetUint("user_id")
	id, _  := strconv.ParseUint(c.Param("id"), 10, 64)
	var req struct {
		Label         *string `json:"label"`
		URL           *string `json:"url"`
		PaymentMethod *string `json:"payment_method"`
		Enabled       *bool   `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid payload")
		return
	}
	updates := map[string]any{}
	if req.Label != nil         { updates["label"] = *req.Label }
	if req.URL != nil           { updates["url"] = *req.URL }
	if req.PaymentMethod != nil { updates["payment_method"] = *req.PaymentMethod }
	if req.Enabled != nil       { updates["enabled"] = *req.Enabled }
	if len(updates) == 0 {
		response.Fail(c, http.StatusBadRequest, "nothing to update")
		return
	}
	if err := h.DB.Model(&model.WebhookEndpoint{}).Where("id = ? AND user_id = ?", id, userID).Updates(updates).Error; err != nil {
		response.Fail(c, http.StatusInternalServerError, "update failed")
		return
	}
	response.OK(c, gin.H{"id": id, "updated": true})
}

func (h *Handler) DeleteWebhook(c *gin.Context) {
	userID := c.GetUint("user_id")
	id, _  := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := h.DB.Where("id = ? AND user_id = ?", id, userID).Delete(&model.WebhookEndpoint{}).Error; err != nil {
		response.Fail(c, http.StatusInternalServerError, "delete webhook failed")
		return
	}
	response.OK(c, gin.H{"id": id, "deleted": true})
}

func (h *Handler) WebhookConfigString(c *gin.Context) {
	userID := c.GetUint("user_id")
	id, _  := strconv.ParseUint(c.Param("id"), 10, 64)
	var ep model.WebhookEndpoint
	if err := h.DB.Where("id = ? AND user_id = ?", id, userID).First(&ep).Error; err != nil {
		response.Fail(c, http.StatusNotFound, "endpoint not found")
		return
	}
	var cs string
	if ep.Type == "a" {
		cs = buildConfigString("a", h.BaseURL, map[string]string{"ak": ep.AApiKey, "whsec": ep.SharedSecret})
		response.OK(c, gin.H{"config_string": cs, "type": "a",
			"callback_url": strings.TrimRight(h.BaseURL, "/") + "/api/gateway/callback"})
	} else {
		cs = buildConfigString("b", h.BaseURL, map[string]string{"bk": ep.BApiKey, "bsk": ep.BSharedSecret})
		response.OK(c, gin.H{"config_string": cs, "type": "b"})
	}
}

// ── helpers ───────────────────────────────────────────

func buildConfigString(epType, nmeBase string, keys map[string]string) string {
	data := map[string]string{"v": "2", "type": epType, "nme": strings.TrimRight(nmeBase, "/")}
	for k, v := range keys {
		data[k] = v
	}
	b, _ := json.Marshal(data)
	return hex.EncodeToString(b) // base64 alternative using hex for simplicity
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
