package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"nme-v9/internal/model"
	"nme-v9/internal/pkg/response"
	"nme-v9/internal/service"
)

// ════════════════════════════════════════════════════════
// SmartRouting 智能路由
// ════════════════════════════════════════════════════════

func (h *Handler) SmartRoutingStats(c *gin.Context) {
	svc := &service.SmartRoutingService{DB: h.DB, RDB: nil, Logger: h.Log}
	var paypalAccounts []model.PaypalAccount
	var stripeConfigs []model.StripeConfig
	h.DB.Where("enabled = true AND account_state = 'active'").Find(&paypalAccounts)
	h.DB.Where("enabled = true AND account_state = 'active'").Find(&stripeConfigs)

	var snapshots []service.WeightSnapshot
	for _, a := range paypalAccounts {
		sw := svc.GetDynamicWeight("paypal", int64(a.ID))
		snapshots = append(snapshots, service.WeightSnapshot{
			ChannelType:  "paypal",
			ChannelID:    int64(a.ID),
			Label:        a.Label,
			StaticWeight: a.Weight,
			SmartWeight:  sw,
			FinalWeight:  float64(a.Weight)*0.4 + sw*0.6,
		})
	}
	for _, s := range stripeConfigs {
		sw := svc.GetDynamicWeight("stripe", int64(s.ID))
		snapshots = append(snapshots, service.WeightSnapshot{
			ChannelType:  "stripe",
			ChannelID:    int64(s.ID),
			Label:        s.Label,
			StaticWeight: s.Weight,
			SmartWeight:  sw,
			FinalWeight:  float64(s.Weight)*0.4 + sw*0.6,
		})
	}
	response.OK(c, gin.H{"snapshots": snapshots})
}

func (h *Handler) SmartRoutingRecalculate(c *gin.Context) {
	svc := &service.SmartRoutingService{DB: h.DB, Logger: h.Log}
	go svc.RecalculateAll()
	response.OK(c, gin.H{"message": "recalculation started (async)"})
}

// ════════════════════════════════════════════════════════
// Channel Metrics
// ════════════════════════════════════════════════════════

func (h *Handler) ChannelMetrics(c *gin.Context) {
	channelType := c.Query("channel_type") // paypal|stripe
	hoursStr := c.DefaultQuery("hours", "24")
	hours, _ := strconv.Atoi(hoursStr)
	if hours <= 0 || hours > 168 {
		hours = 24
	}

	since := time.Now().UTC().Add(-time.Duration(hours) * time.Hour).Truncate(time.Hour)
	query := h.DB.Where("hour_slot >= ?", since).Order("hour_slot desc")
	if channelType != "" {
		query = query.Where("channel_type = ?", channelType)
	}

	var items []model.ChannelMetric
	if err := query.Limit(500).Find(&items).Error; err != nil {
		response.Fail(c, http.StatusInternalServerError, "query failed")
		return
	}
	response.OK(c, gin.H{"items": items, "hours": hours})
}

// ════════════════════════════════════════════════════════
// Alert 告警（升级版）
// ════════════════════════════════════════════════════════

func (h *Handler) Alerts(c *gin.Context) {
	status := c.Query("status")
	level := c.Query("level")
	alertType := c.Query("type")

	query := h.DB.Model(&model.AlertRecord{}).Order("id desc").Limit(200)
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if level != "" {
		query = query.Where("level = ?", level)
	}
	if alertType != "" {
		query = query.Where("type = ?", alertType)
	}

	var items []model.AlertRecord
	if err := query.Find(&items).Error; err != nil {
		response.Fail(c, http.StatusInternalServerError, "query failed")
		return
	}
	response.OK(c, gin.H{"items": items})
}

func (h *Handler) AcknowledgeAlert(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	userID := c.GetUint("user_id")
	byWho := fmt.Sprintf("admin:%d", userID)
	svc := &service.AlertService{DB: h.DB, Log: h.Log}
	if err := svc.Acknowledge(uint(id), byWho); err != nil {
		response.Fail(c, http.StatusInternalServerError, "acknowledge failed")
		return
	}
	response.OK(c, gin.H{"id": id, "status": "acknowledged"})
}

func (h *Handler) ResolveAlert(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	svc := &service.AlertService{DB: h.DB, Log: h.Log}
	if err := svc.Resolve(uint(id)); err != nil {
		response.Fail(c, http.StatusInternalServerError, "resolve failed")
		return
	}
	response.OK(c, gin.H{"id": id, "status": "resolved"})
}

// ════════════════════════════════════════════════════════
// Alert Channels 告警渠道
// ════════════════════════════════════════════════════════

func (h *Handler) AlertChannels(c *gin.Context) {
	svc := &service.AlertService{DB: h.DB, Log: h.Log}
	response.OK(c, gin.H{"items": svc.GetChannelList()})
}

func (h *Handler) CreateAlertChannel(c *gin.Context) {
	var req struct {
		Name   string          `json:"name"`
		Type   string          `json:"type"`
		Config json.RawMessage `json:"config"`
		Levels []string        `json:"levels"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Type == "" {
		response.Fail(c, http.StatusBadRequest, "type required (telegram|webhook|email)")
		return
	}
	if req.Type != "telegram" && req.Type != "webhook" && req.Type != "email" {
		response.Fail(c, http.StatusBadRequest, "type must be telegram|webhook|email")
		return
	}

	cfgStr := "{}"
	if req.Config != nil {
		cfgStr = string(req.Config)
	}
	levelsStr := `["all"]`
	if len(req.Levels) > 0 {
		if b, err := json.Marshal(req.Levels); err == nil {
			levelsStr = string(b)
		}
	}

	item := model.AlertChannel{
		Name:    req.Name,
		Type:    req.Type,
		Config:  cfgStr,
		Levels:  levelsStr,
		Enabled: true,
	}
	if err := h.DB.Create(&item).Error; err != nil {
		response.Fail(c, http.StatusBadRequest, "create failed")
		return
	}
	response.OK(c, gin.H{"id": item.ID, "type": item.Type, "name": item.Name})
}

func (h *Handler) DeleteAlertChannel(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := h.DB.Delete(&model.AlertChannel{}, id).Error; err != nil {
		response.Fail(c, http.StatusInternalServerError, "delete failed")
		return
	}
	response.OK(c, gin.H{"id": id, "deleted": true})
}

func (h *Handler) TestAlertPush(c *gin.Context) {
	svc := &service.AlertService{DB: h.DB, Log: h.Log}
	svc.TestPush()
	response.OK(c, gin.H{"message": "test alert sent to all enabled channels"})
}

// ════════════════════════════════════════════════════════
// Risk Rules（升级版，支持 type/conditions 新字段）
// ════════════════════════════════════════════════════════

func (h *Handler) RiskRules(c *gin.Context) {
	ruleType := c.Query("type")
	query := h.DB.Model(&model.RiskRule{}).Order("id asc")
	if ruleType != "" {
		query = query.Where("type = ?", ruleType)
	}
	var items []model.RiskRule
	if err := query.Find(&items).Error; err != nil {
		response.Fail(c, http.StatusInternalServerError, "query failed")
		return
	}
	response.OK(c, gin.H{"items": items})
}

func (h *Handler) CreateRiskRule(c *gin.Context) {
	var req struct {
		Name        string          `json:"name"`
		Type        string          `json:"type"`
		Enabled     bool            `json:"enabled"`
		Action      string          `json:"action"`
		RiskScore   int             `json:"risk_score"`
		Conditions  json.RawMessage `json:"conditions"`
		Config      json.RawMessage `json:"config"`
		Description string          `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
		response.Fail(c, http.StatusBadRequest, "name required")
		return
	}
	if req.Type == "" {
		req.Type = "amount_range"
	}
	if req.Action != "warn" {
		req.Action = "block"
	}
	if req.RiskScore <= 0 {
		req.RiskScore = 20
	}

	condStr := "{}"
	if req.Conditions != nil {
		condStr = string(req.Conditions)
	}
	cfgStr := "{}"
	if req.Config != nil {
		cfgStr = string(req.Config)
	}

	item := model.RiskRule{
		Name:        req.Name,
		Type:        req.Type,
		Enabled:     req.Enabled,
		Action:      req.Action,
		RiskScore:   req.RiskScore,
		Conditions:  condStr,
		Config:      cfgStr,
		Description: req.Description,
	}
	if err := h.DB.Create(&item).Error; err != nil {
		response.Fail(c, http.StatusBadRequest, "create failed")
		return
	}
	response.OK(c, item)
}

func (h *Handler) UpdateRiskRule(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req map[string]any
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid payload")
		return
	}
	// 禁止覆盖统计字段
	delete(req, "id")
	delete(req, "hit_count")
	if err := h.DB.Model(&model.RiskRule{}).Where("id = ?", id).Updates(req).Error; err != nil {
		response.Fail(c, http.StatusInternalServerError, "update failed")
		return
	}
	response.OK(c, gin.H{"id": id, "updated": true})
}

func (h *Handler) DeleteRiskRule(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	if err := h.DB.Delete(&model.RiskRule{}, id).Error; err != nil {
		response.Fail(c, http.StatusInternalServerError, "delete failed")
		return
	}
	response.OK(c, gin.H{"id": id, "deleted": true})
}

func (h *Handler) ToggleRiskRule(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var item model.RiskRule
	if err := h.DB.First(&item, id).Error; err != nil {
		response.Fail(c, http.StatusNotFound, "not found")
		return
	}
	newEnabled := !item.Enabled
	h.DB.Model(&item).Update("enabled", newEnabled)
	response.OK(c, gin.H{"id": id, "enabled": newEnabled})
}
