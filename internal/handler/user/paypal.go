package user

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"nme-v9/internal/model"
	"nme-v9/internal/pkg/response"
)

// ════════════════════════════════════════════════════════
// PayPal 账号管理
// ════════════════════════════════════════════════════════

func (h *Handler) PaypalAccounts(c *gin.Context) {
	userID := c.GetUint("user_id")
	var items []model.PaypalAccount
	if err := h.DB.Where("user_id = ?", userID).Order("poll_order asc, id asc").Find(&items).Error; err != nil {
		response.Fail(c, http.StatusInternalServerError, "query failed")
		return
	}
	response.OK(c, gin.H{"items": items})
}

func (h *Handler) CreatePaypalAccount(c *gin.Context) {
	userID := c.GetUint("user_id")
	var req struct {
		Label              string  `json:"label"`
		Mode               string  `json:"mode"`
		Email              string  `json:"email"`
		PaypalMeUsername   string  `json:"paypalme_username"`
		ClientID           string  `json:"client_id"`
		ClientSecret       string  `json:"client_secret"`
		Sandbox            bool    `json:"sandbox"`
		SandboxMode        bool    `json:"sandbox_mode"`
		SandboxEmail       string  `json:"sandbox_email"`
		SandboxPaypalMeUsername string `json:"sandbox_paypalme_username"`
		SandboxClientID    string  `json:"sandbox_client_id"`
		SandboxClientSecret string `json:"sandbox_client_secret"`
		PollMode           string  `json:"poll_mode"`
		PollOrder          *int    `json:"poll_order"`
		Weight             int     `json:"weight"`
		MinAmount          float64 `json:"min_amount"`
		MaxAmount          float64 `json:"max_amount"`
		MaxOrders          int     `json:"max_orders"`
		MaxAmountTotal     float64 `json:"max_amount_total"`
		DailyResetHour     int     `json:"daily_reset_hour"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid payload")
		return
	}
	if req.Mode != "rest" {
		req.Mode = "email"
	}
	if req.PollMode != "sequence" {
		req.PollMode = "random"
	}
	if req.Weight <= 0 {
		req.Weight = 10
	}
	if req.MaxAmount <= 0 {
		req.MaxAmount = 99999
	}
	if req.DailyResetHour < 0 || req.DailyResetHour > 23 {
		req.DailyResetHour = 0
	}

	// Auto poll_order
	var maxOrder int
	h.DB.Model(&model.PaypalAccount{}).Where("user_id = ?", userID).Select("COALESCE(MAX(poll_order), -1)").Scan(&maxOrder)
	pollOrder := maxOrder + 1
	if req.PollOrder != nil && *req.PollOrder >= 0 {
		pollOrder = *req.PollOrder
	}

	item := model.PaypalAccount{
		UserID:              userID,
		Label:               req.Label,
		Mode:                req.Mode,
		Email:               req.Email,
		PaypalMeUsername:    req.PaypalMeUsername,
		ClientID:            req.ClientID,
		ClientSecret:        req.ClientSecret,
		Sandbox:             req.Sandbox,
		SandboxMode:         req.SandboxMode,
		SandboxEmail:        req.SandboxEmail,
		SandboxPaypalMeUsername: req.SandboxPaypalMeUsername,
		SandboxClientID:     req.SandboxClientID,
		SandboxClientSecret: req.SandboxClientSecret,
		AccountState:        "active",
		Enabled:             true,
		PollMode:            req.PollMode,
		PollOrder:           pollOrder,
		Weight:              req.Weight,
		SmartWeight:         50.0,
		MinAmount:           req.MinAmount,
		MaxAmount:           req.MaxAmount,
		MaxOrders:           req.MaxOrders,
		MaxAmountTotal:      req.MaxAmountTotal,
		DailyResetHour:      req.DailyResetHour,
	}
	if err := h.DB.Create(&item).Error; err != nil {
		response.Fail(c, http.StatusBadRequest, "create failed")
		return
	}
	response.OK(c, item)
}

func (h *Handler) UpdatePaypalAccount(c *gin.Context) {
	userID := c.GetUint("user_id")
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req map[string]any
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid payload")
		return
	}
	// 禁止修改受保护字段
	for _, k := range []string{"id", "user_id", "total_orders", "total_success", "fail_count",
		"daily_orders", "daily_amount", "smart_weight"} {
		delete(req, k)
	}
	if err := h.DB.Model(&model.PaypalAccount{}).Where("id = ? AND user_id = ?", id, userID).Updates(req).Error; err != nil {
		response.Fail(c, http.StatusInternalServerError, "update failed")
		return
	}
	response.OK(c, gin.H{"id": id, "updated": true})
}

func (h *Handler) DeletePaypalAccount(c *gin.Context) {
	userID := c.GetUint("user_id")
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := h.DB.Where("id = ? AND user_id = ?", id, userID).Delete(&model.PaypalAccount{}).Error; err != nil {
		response.Fail(c, http.StatusInternalServerError, "delete failed")
		return
	}
	response.OK(c, gin.H{"id": id, "deleted": true})
}

func (h *Handler) TogglePaypalAccount(c *gin.Context) {
	userID := c.GetUint("user_id")
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var item model.PaypalAccount
	if err := h.DB.Where("id = ? AND user_id = ?", id, userID).First(&item).Error; err != nil {
		response.Fail(c, http.StatusNotFound, "not found")
		return
	}
	newEnabled := !item.Enabled
	h.DB.Model(&item).Update("enabled", newEnabled)
	response.OK(c, gin.H{"id": id, "enabled": newEnabled})
}

func (h *Handler) ResetPaypalDaily(c *gin.Context) {
	userID := c.GetUint("user_id")
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := h.DB.Model(&model.PaypalAccount{}).Where("id = ? AND user_id = ?", id, userID).Updates(map[string]any{
		"daily_orders": 0,
		"daily_amount": 0,
	}).Error; err != nil {
		response.Fail(c, http.StatusInternalServerError, "reset failed")
		return
	}
	response.OK(c, gin.H{"id": id, "reset": true})
}

// SetPaypalAccountState 设置账号三态（active/paused/abandoned）
func (h *Handler) SetPaypalAccountState(c *gin.Context) {
	userID := c.GetUint("user_id")
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req struct {
		State string `json:"state"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || (req.State != "active" && req.State != "paused" && req.State != "abandoned") {
		response.Fail(c, http.StatusBadRequest, "state must be active|paused|abandoned")
		return
	}
	updates := map[string]any{"account_state": req.State}
	if req.State == "abandoned" {
		updates["enabled"] = false
	}
	h.DB.Model(&model.PaypalAccount{}).Where("id = ? AND user_id = ?", id, userID).Updates(updates)
	response.OK(c, gin.H{"id": id, "account_state": req.State})
}

