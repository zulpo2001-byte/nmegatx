package user

// orders_stats.go — 用户侧订单统计
//
// GET /api/user/orders/stats — 当前用户订单的状态分布 + 收入汇总

import (
	"time"

	"github.com/gin-gonic/gin"
	"nme-v9/internal/model"
	"nme-v9/internal/pkg/response"
)

// UserOrdersStats GET /api/user/orders/stats
func (h *Handler) UserOrdersStats(c *gin.Context) {
	userID := c.GetUint("user_id")

	type row struct {
		Status string  `json:"status"`
		Count  int64   `json:"count"`
		Amount float64 `json:"amount"`
	}
	var rows []row
	h.DB.Raw(`
		SELECT status,
		       COUNT(*) AS count,
		       COALESCE(SUM(amount), 0) AS amount
		FROM orders
		WHERE user_id = ?
		GROUP BY status
	`, userID).Scan(&rows)

	// 今日数据
	today := time.Now().UTC().Truncate(24 * time.Hour)
	var todayOrders int64
	var todayRevenue float64
	h.DB.Model(&model.Order{}).Where("user_id = ? AND created_at >= ?", userID, today).Count(&todayOrders)
	h.DB.Model(&model.Order{}).
		Where("user_id = ? AND status = ? AND created_at >= ?", userID, "completed", today).
		Select("COALESCE(SUM(amount), 0)").Scan(&todayRevenue)

	// 成功率
	var totalOrders, completedOrders int64
	h.DB.Model(&model.Order{}).Where("user_id = ?", userID).Count(&totalOrders)
	h.DB.Model(&model.Order{}).Where("user_id = ? AND status = ?", userID, "completed").Count(&completedOrders)
	successRate := 0.0
	if totalOrders > 0 {
		successRate = float64(completedOrders) / float64(totalOrders) * 100
	}

	response.OK(c, gin.H{
		"by_status":      rows,
		"total_orders":   totalOrders,
		"today_orders":   todayOrders,
		"today_revenue":  todayRevenue,
		"success_rate":   successRate,
	})
}
