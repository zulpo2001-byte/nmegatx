package admin

// orders_extra.go — 订单运维接口
//
// GET    /api/admin/orders/:id          — 单条订单详情
// GET    /api/admin/orders/reset-mode   — 查询账号每日重置模式
// POST   /api/admin/orders/force-reset  — 强制立即重置所有账号的今日计数

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"nme-v9/internal/model"
	"nme-v9/internal/pkg/response"
)

// OrderDetail GET /api/admin/orders/:id
func (h *Handler) OrderDetail(c *gin.Context) {
	id, _ := strconv.ParseUint(c.Param("id"), 10, 64)
	var order model.Order
	if err := h.DB.First(&order, id).Error; err != nil {
		response.Fail(c, http.StatusNotFound, "order not found")
		return
	}
	response.OK(c, order)
}

// GetResetMode GET /api/admin/orders/reset-mode
// 返回当前账号每日计数的重置模式（daily=凌晨自动, manual=手动）
func (h *Handler) GetResetMode(c *gin.Context) {
	var s model.GlobalSetting
	mode := "daily"
	if h.DB.Where("key = ?", "reset_mode").First(&s).Error == nil {
		mode = s.Value
	}
	response.OK(c, gin.H{"reset_mode": mode})
}

// ForceReset POST /api/admin/orders/force-reset
// 强制将所有 paypal_accounts 和 stripe_configs 的 daily_orders/daily_amount 归零
func (h *Handler) ForceReset(c *gin.Context) {
	now := time.Now().UTC()
	todayDate, _ := time.Parse("2006-01-02", now.Format("2006-01-02"))

	res1 := h.DB.Exec("UPDATE paypal_accounts SET daily_orders=0, daily_amount=0, last_reset_date=?", todayDate)
	res2 := h.DB.Exec("UPDATE stripe_configs  SET daily_orders=0, daily_amount=0, last_reset_date=?", todayDate)

	if res1.Error != nil || res2.Error != nil {
		response.Fail(c, http.StatusInternalServerError, "force reset failed")
		return
	}
	response.OK(c, gin.H{
		"reset_at":          now.Format(time.RFC3339),
		"paypal_rows_reset": res1.RowsAffected,
		"stripe_rows_reset": res2.RowsAffected,
	})
}
