package admin

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"nmegateway/internal/model"
	"nmegateway/internal/pkg/response"
)

// Handler 管理后台 handler，持有 DB、Redis、Logger
type Handler struct {
	DB  *gorm.DB
	Log *zap.Logger
	RDB *redis.Client // 用于 MetricsSummary / MetricsResetEndpoint
}

// Stats GET /api/admin/stats — 系统概览统计
func (h *Handler) Stats(c *gin.Context) {
	var users, orders, pending, completed int64
	h.DB.Model(&model.User{}).Count(&users)
	h.DB.Model(&model.Order{}).Count(&orders)
	h.DB.Model(&model.Order{}).Where("status = ?", "pending").Count(&pending)
	h.DB.Model(&model.Order{}).Where("status = ?", "completed").Count(&completed)

	// 今日数据
	today := time.Now().UTC().Truncate(24 * time.Hour)
	var todayOrders int64
	var todayRevenue float64
	h.DB.Model(&model.Order{}).Where("created_at >= ?", today).Count(&todayOrders)
	h.DB.Model(&model.Order{}).
		Where("status = ? AND created_at >= ?", "completed", today).
		Select("COALESCE(SUM(amount),0)").Scan(&todayRevenue)

	// 用户状态分布
	var activeUsers, suspendedUsers int64
	h.DB.Model(&model.User{}).Where("status = ?", "active").Count(&activeUsers)
	h.DB.Model(&model.User{}).Where("status = ?", "suspended").Count(&suspendedUsers)

	response.OK(c, gin.H{
		"users":           users,
		"active_users":    activeUsers,
		"suspended_users": suspendedUsers,
		"orders":          orders,
		"pending":         pending,
		"completed":       completed,
		"today_orders":    todayOrders,
		"today_revenue":   todayRevenue,
	})
}

// Roles GET /api/admin/roles
func (h *Handler) Roles(c *gin.Context) {
	var items []model.Role
	if err := h.DB.Order("id desc").Find(&items).Error; err != nil {
		response.Fail(c, http.StatusInternalServerError, "query roles failed")
		return
	}
	response.OK(c, gin.H{"items": items})
}

// Orders GET /api/admin/orders?days=N&status=S
func (h *Handler) Orders(c *gin.Context) {
	daysStr := c.Query("days")
	status := c.Query("status")
	paymentMethod := c.Query("payment_method")

	query := h.DB.Order("id desc").Limit(500)
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
	if paymentMethod != "" {
		query = query.Where("payment_method = ?", paymentMethod)
	}

	var items []model.Order
	if err := query.Find(&items).Error; err != nil {
		response.Fail(c, http.StatusInternalServerError, "query orders failed")
		return
	}
	response.OK(c, gin.H{"items": items, "count": len(items)})
}

// AuditLogs GET /api/admin/audit-logs
// 支持过滤：?actor_id=N&action=xxx&method=POST&days=7&page=1&page_size=50
func (h *Handler) AuditLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 50
	}
	offset := (page - 1) * pageSize

	query := h.DB.Model(&model.AuditLog{}).Order("id desc")

	if v := c.Query("actor_id"); v != "" {
		query = query.Where("actor_id = ?", v)
	}
	if v := c.Query("action"); v != "" {
		query = query.Where("action ILIKE ?", "%"+v+"%")
	}
	if v := c.Query("method"); v != "" {
		query = query.Where("method = ?", v)
	}
	if v := c.Query("days"); v != "" {
		if d, err := strconv.Atoi(v); err == nil && d > 0 {
			since := time.Now().UTC().AddDate(0, 0, -d)
			query = query.Where("created_at >= ?", since)
		}
	}

	var total int64
	query.Count(&total)

	var items []model.AuditLog
	if err := query.Limit(pageSize).Offset(offset).Find(&items).Error; err != nil {
		response.Fail(c, http.StatusInternalServerError, "query audit logs failed")
		return
	}
	response.OK(c, gin.H{
		"items":     items,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}
