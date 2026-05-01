package user

import (
	"github.com/gin-gonic/gin"
	"nmegateway/internal/model"
	"nmegateway/internal/pkg/response"
)

func (h *Handler) Dashboard(c *gin.Context) {
	userID := c.GetUint("user_id")

	// 订单统计
	var total, pending, completed, expired int64
	h.DB.Model(&model.Order{}).Where("user_id = ?", userID).Count(&total)
	h.DB.Model(&model.Order{}).Where("user_id = ? AND status = ?", userID, "pending").Count(&pending)
	h.DB.Model(&model.Order{}).Where("user_id = ? AND status = ?", userID, "completed").Count(&completed)
	h.DB.Model(&model.Order{}).Where("user_id = ? AND status IN (?)", userID, []string{"expired", "abandoned"}).Count(&expired)
	successRate := 0.0
	if total > 0 {
		successRate = float64(completed) * 100 / float64(total)
	}

	// 当前用户完整信息（含到期时间、轮询策略）
	var user model.User
	h.DB.First(&user, userID)

	// PayPal 账号数
	var paypalCount, stripeCount int64
	h.DB.Model(&model.PaypalAccount{}).Where("user_id = ? AND enabled = true AND account_state = 'active'", userID).Count(&paypalCount)
	h.DB.Model(&model.StripeConfig{}).Where("user_id = ? AND enabled = true AND account_state = 'active'", userID).Count(&stripeCount)

	response.OK(c, gin.H{
		"orders_total":     total,
		"pending":          pending,
		"completed":        completed,
		"expired":          expired,
		"success_rate":     successRate,
		"expires_at":       user.ExpiresAt,
		"paypal_count":     paypalCount,
		"stripe_count":     stripeCount,
		"user_id":          user.ID,
	})
}
