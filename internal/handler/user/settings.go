package user

// settings.go — 用户自服务：只读设置 + 权重快照
//
// 密码修改统一在 PUT /api/profile/password（admin.UpdateSelfPassword）处理，
// handler 内部按 role 区分：super_admin 可改邮箱，其余角色只能改密码。
//
// GET /api/user/settings                 — 读取对当前用户生效的全局配置（只读）
// GET /api/user/smart-routing/my-weights — 自己名下账号的当前权重快照（只读）

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"nme-v9/internal/model"
	"nme-v9/internal/pkg/response"
	"nme-v9/internal/service"
)

// UserReadSettings GET /api/user/settings
// 返回对当前用户生效的全局设置（风控阈值、智能路由开关等），只读展示
func (h *Handler) UserReadSettings(c *gin.Context) {
	keys := []string{
		"risk_block_threshold",
		"risk_warn_threshold",
		"smart_routing_enabled",
		"smart_routing_w_success",
		"smart_routing_w_risk",
		"smart_routing_w_resp",
		"wa_number",
		"wa_message",
	}
	defaults := map[string]string{
		"risk_block_threshold":    "70",
		"risk_warn_threshold":     "40",
		"smart_routing_enabled":   "1",
		"smart_routing_w_success": "0.50",
		"smart_routing_w_risk":    "0.30",
		"smart_routing_w_resp":    "0.20",
		"wa_number":               "",
		"wa_message":              "网络繁忙，请联系客服",
	}

	var items []model.GlobalSetting
	h.DB.Where("key IN ?", keys).Find(&items)

	m := map[string]string{}
	for _, s := range items {
		m[s.Key] = s.Value
	}
	for _, k := range keys {
		if _, exists := m[k]; !exists {
			m[k] = defaults[k]
		}
	}
	response.OK(c, gin.H{"settings": m})
}

// UserSmartRoutingWeights GET /api/user/smart-routing/my-weights
// 返回当前用户名下所有活跃账号的动态权重快照（只读，不含敏感 key）
func (h *Handler) UserSmartRoutingWeights(c *gin.Context) {
	userID := c.GetUint("user_id")
	svc := &service.SmartRoutingService{DB: h.DB, RDB: nil, Logger: nil}
	snapshots := svc.GetWeightSnapshots(userID)
	if snapshots == nil {
		snapshots = []service.WeightSnapshot{}
	}
	response.OK(c, gin.H{
		"user_id":   userID,
		"snapshots": snapshots,
		"note":      "final_weight = static_weight×0.4 + smart_weight×0.6",
	})
}

// unused import guard — keep http used by response.Fail if needed in future
var _ = http.StatusOK
