package admin

// settings.go — 系统设置 + super_admin 专属账号运维

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"nmegateway/internal/model"
	"nmegateway/internal/pkg/response"
)

// settingKeys 白名单：所有允许写入的全局设置 key
var settingKeys = map[string]bool{
	// ── 原有字段 ──────────────────────────────────────────
	"reset_mode":       true,
	"max_account_hops": true,
	"redis_lock_ttl":   true,
	"ghost_order_ttl":  true,
	// ── 风控阈值（SaaS 迁移）─────────────────────────────
	"risk_block_threshold": true,
	"risk_warn_threshold":  true,
	"chargeback_threshold": true,
	"risk_keywords":        true,
	// ── 智能路由权重系数（SaaS 迁移）────────────────────
	"smart_routing_enabled":   true,
	"smart_routing_w_success": true,
	"smart_routing_w_risk":    true,
	"smart_routing_w_resp":    true,
	// ── 客服配置（SaaS 迁移）─────────────────────────────
	"wa_number":  true,
	"wa_message": true,
}

var settingDefaults = map[string]string{
	"reset_mode":              "daily",
	"max_account_hops":        "10",
	"redis_lock_ttl":          "5",
	"ghost_order_ttl":         "30",
	"risk_block_threshold":    "70",
	"risk_warn_threshold":     "40",
	"chargeback_threshold":    "3",
	"risk_keywords":           "ACCOUNT_RESTRICTED,charges_disabled",
	"smart_routing_enabled":   "1",
	"smart_routing_w_success": "0.50",
	"smart_routing_w_risk":    "0.30",
	"smart_routing_w_resp":    "0.20",
	"wa_number":               "",
	"wa_message":              "网络繁忙，请联系客服",
}

// GetSettings GET /api/admin/settings
// 读取所有全局设置，补充未配置项的默认值
func (h *Handler) GetSettings(c *gin.Context) {
	var items []model.GlobalSetting
	if err := h.DB.Order("key asc").Find(&items).Error; err != nil {
		response.Fail(c, http.StatusInternalServerError, "query settings failed")
		return
	}
	m := map[string]string{}
	for _, s := range items {
		m[s.Key] = s.Value
	}
	for k, def := range settingDefaults {
		if _, exists := m[k]; !exists {
			m[k] = def
		}
	}
	response.OK(c, gin.H{"settings": m})
}

// UpdateSettings POST /api/admin/settings
// Body: {"risk_block_threshold":"80","smart_routing_enabled":"1",...}
func (h *Handler) UpdateSettings(c *gin.Context) {
	var req map[string]string
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid payload: expect {key:value,...}")
		return
	}
	var updated []string
	for k, v := range req {
		if !settingKeys[k] {
			continue
		}
		var s model.GlobalSetting
		if h.DB.Where("key = ?", k).First(&s).Error == nil {
			h.DB.Model(&s).Update("value", v)
		} else {
			h.DB.Create(&model.GlobalSetting{Key: k, Value: v})
		}
		updated = append(updated, k)
	}
	response.OK(c, gin.H{"updated": updated, "count": len(updated)})
}

// ════════════════════════════════════════════════════════
// super_admin 专属：账号运维（TestAlertPush 已在 channels.go 中实现）
// ════════════════════════════════════════════════════════

// AdminResetPassword POST /api/admin/users/:id/reset-password
// super_admin 强制重置任意用户密码，不需要旧密码
func (h *Handler) AdminResetPassword(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req struct {
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Password) < 6 {
		response.Fail(c, http.StatusBadRequest, "password required (min 6 chars)")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, "password hash failed")
		return
	}
	res := h.DB.Model(&model.User{}).Where("id = ?", id).Update("password", string(hash))
	if res.Error != nil {
		response.Fail(c, http.StatusInternalServerError, "reset password failed")
		return
	}
	if res.RowsAffected == 0 {
		response.Fail(c, http.StatusNotFound, "user not found")
		return
	}
	response.OK(c, gin.H{"id": id, "reset": true})
}

// AdminToggleStatus POST /api/admin/users/:id/toggle-status
// 切换用户 active ↔ suspended
func (h *Handler) AdminToggleStatus(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var u model.User
	if err := h.DB.First(&u, id).Error; err != nil {
		response.Fail(c, http.StatusNotFound, "user not found")
		return
	}
	newStatus := "suspended"
	if u.Status == "suspended" {
		newStatus = "active"
	}
	h.DB.Model(&u).Update("status", newStatus)
	response.OK(c, gin.H{"id": id, "status": newStatus})
}

// AdminUpdateUserPermissions PUT /api/admin/users/:id/permissions
// super_admin 设置用户的细粒度权限 JSON
func (h *Handler) AdminUpdateUserPermissions(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var req struct {
		Permissions map[string]bool `json:"permissions"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid payload")
		return
	}
	b, err := json.Marshal(req.Permissions)
	if err != nil {
		response.Fail(c, http.StatusInternalServerError, "marshal permissions failed")
		return
	}
	res := h.DB.Model(&model.User{}).Where("id = ?", id).Update("permissions", string(b))
	if res.RowsAffected == 0 {
		response.Fail(c, http.StatusNotFound, "user not found")
		return
	}
	response.OK(c, gin.H{"id": id, "permissions": req.Permissions})
}
