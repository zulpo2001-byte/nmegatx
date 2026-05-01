package admin

// reports.go — 报表导出
//
// GET /api/admin/reports/daily?date=YYYY-MM-DD&user_id=N
//     按单日汇总：总订单数/完成数/失败数/收入/成功率/按小时分布/按通道分布/风险统计
//
// GET /api/admin/reports/export?from=YYYY-MM-DD&to=YYYY-MM-DD&format=csv
//     区间导出（默认 CSV，也可 format=json）

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"nmegateway/internal/model"
	"nmegateway/internal/pkg/response"
)

// ReportsDaily GET /api/admin/reports/daily
func (h *Handler) ReportsDaily(c *gin.Context) {
	dateStr := c.DefaultQuery("date", time.Now().UTC().Format("2006-01-02"))
	userIDStr := c.Query("user_id")

	date, err := time.ParseInLocation("2006-01-02", dateStr, time.UTC)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid date format, use YYYY-MM-DD")
		return
	}
	dayStart := date.Truncate(24 * time.Hour)
	dayEnd := dayStart.Add(24 * time.Hour)

	base := h.DB.Model(&model.Order{}).Where("created_at >= ? AND created_at < ?", dayStart, dayEnd)
	if userIDStr != "" {
		if uid, err := strconv.ParseUint(userIDStr, 10, 64); err == nil {
			base = base.Where("user_id = ?", uid)
		}
	}

	// ── 汇总 ──────────────────────────────────────────────
	type countRow struct {
		Status string
		Count  int64
	}
	var counts []countRow
	h.DB.Model(&model.Order{}).
		Select("status, count(*) as count").
		Where("created_at >= ? AND created_at < ?", dayStart, dayEnd).
		Group("status").
		Scan(&counts)

	summary := map[string]any{
		"date":             dateStr,
		"total_orders":     int64(0),
		"completed_orders": int64(0),
		"failed_orders":    int64(0),
		"pending_orders":   int64(0),
		"total_revenue":    float64(0),
		"success_rate":     float64(0),
	}
	for _, row := range counts {
		n := row.Count
		summary["total_orders"] = summary["total_orders"].(int64) + n
		switch row.Status {
		case "completed":
			summary["completed_orders"] = n
		case "failed", "abandoned":
			summary["failed_orders"] = summary["failed_orders"].(int64) + n
		case "pending":
			summary["pending_orders"] = n
		}
	}
	total := summary["total_orders"].(int64)
	completed := summary["completed_orders"].(int64)
	if total > 0 {
		summary["success_rate"] = float64(completed) / float64(total) * 100
	}

	// 总收入
	var revenue float64
	h.DB.Model(&model.Order{}).
		Where("status = ? AND created_at >= ? AND created_at < ?", "completed", dayStart, dayEnd).
		Select("COALESCE(SUM(amount),0)").Scan(&revenue)
	summary["total_revenue"] = revenue

	// ── 按小时分布 ────────────────────────────────────────
	type hourRow struct {
		Hour      int     `json:"hour"`
		Total     int64   `json:"total"`
		Completed int64   `json:"completed"`
		Revenue   float64 `json:"revenue"`
	}
	var byHour []hourRow
	h.DB.Raw(`
		SELECT
			EXTRACT(HOUR FROM created_at)::int AS hour,
			COUNT(*) AS total,
			SUM(CASE WHEN status='completed' THEN 1 ELSE 0 END) AS completed,
			SUM(CASE WHEN status='completed' THEN amount ELSE 0 END) AS revenue
		FROM orders
		WHERE created_at >= ? AND created_at < ?
		GROUP BY hour
		ORDER BY hour
	`, dayStart, dayEnd).Scan(&byHour)

	// ── 按支付方式分布 ────────────────────────────────────
	type gwRow struct {
		PaymentMethod string  `json:"payment_method"`
		Orders        int64   `json:"orders"`
		Revenue       float64 `json:"revenue"`
	}
	var byGateway []gwRow
	h.DB.Raw(`
		SELECT payment_method, COUNT(*) AS orders, COALESCE(SUM(amount),0) AS revenue
		FROM orders
		WHERE status='completed' AND created_at >= ? AND created_at < ?
		GROUP BY payment_method
	`, dayStart, dayEnd).Scan(&byGateway)

	// ── 按通道标签分布 ────────────────────────────────────
	type chRow struct {
		GatewayLabel string  `json:"gateway_label"`
		Orders       int64   `json:"orders"`
		Revenue      float64 `json:"revenue"`
	}
	var byChannel []chRow
	h.DB.Raw(`
		SELECT gateway_label, COUNT(*) AS orders, COALESCE(SUM(amount),0) AS revenue
		FROM orders
		WHERE status='completed' AND gateway_label != '' AND created_at >= ? AND created_at < ?
		GROUP BY gateway_label
		ORDER BY revenue DESC
	`, dayStart, dayEnd).Scan(&byChannel)

	// ── 风险统计 ──────────────────────────────────────────
	var avgRisk float64
	var highRiskCount int64
	h.DB.Model(&model.Order{}).
		Where("risk_score > 0 AND created_at >= ? AND created_at < ?", dayStart, dayEnd).
		Select("COALESCE(AVG(risk_score),0)").Scan(&avgRisk)
	h.DB.Model(&model.Order{}).
		Where("risk_score >= 70 AND created_at >= ? AND created_at < ?", dayStart, dayEnd).
		Count(&highRiskCount)

	response.OK(c, gin.H{
		"summary":    summary,
		"by_hour":    byHour,
		"by_gateway": byGateway,
		"by_channel": byChannel,
		"risk": gin.H{
			"avg_risk_score": avgRisk,
			"high_risk_count": highRiskCount,
		},
	})
}

// ReportsExport GET /api/admin/reports/export?from=YYYY-MM-DD&to=YYYY-MM-DD&format=csv
func (h *Handler) ReportsExport(c *gin.Context) {
	fromStr := c.DefaultQuery("from", time.Now().UTC().AddDate(0, 0, -7).Format("2006-01-02"))
	toStr := c.DefaultQuery("to", time.Now().UTC().Format("2006-01-02"))
	format := c.DefaultQuery("format", "csv")

	from, err := time.ParseInLocation("2006-01-02", fromStr, time.UTC)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid from date")
		return
	}
	to, err := time.ParseInLocation("2006-01-02", toStr, time.UTC)
	if err != nil {
		response.Fail(c, http.StatusBadRequest, "invalid to date")
		return
	}
	to = to.Add(24 * time.Hour) // 包含 to 当天

	var orders []model.Order
	if err := h.DB.
		Where("created_at >= ? AND created_at < ?", from, to).
		Order("id desc").
		Limit(5000).
		Find(&orders).Error; err != nil {
		response.Fail(c, http.StatusInternalServerError, "query orders failed")
		return
	}

	if format == "json" {
		response.OK(c, gin.H{
			"from":   fromStr,
			"to":     toStr,
			"total":  len(orders),
			"orders": orders,
		})
		return
	}

	// ── CSV 输出 ──────────────────────────────────────────
	filename := fmt.Sprintf("orders_%s_%s.csv", fromStr, toStr)
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))

	w := csv.NewWriter(c.Writer)
	// BOM for Excel compatibility
	_, _ = c.Writer.WriteString("\xEF\xBB\xBF")

	_ = w.Write([]string{
		"ID", "用户ID", "A站订单", "B站订单",
		"金额", "货币", "支付方式", "通道标签",
		"风险分", "状态", "创建时间",
	})
	for _, o := range orders {
		paidAt := ""
		if o.PaidAt != nil {
			paidAt = o.PaidAt.Format(time.RFC3339)
		}
		_ = w.Write([]string{
			strconv.FormatUint(uint64(o.ID), 10),
			strconv.FormatUint(uint64(o.UserID), 10),
			o.AOrderID,
			o.BOrderID,
			fmt.Sprintf("%.2f", o.Amount),
			o.Currency,
			o.PaymentMethod,
			o.GatewayLabel,
			strconv.Itoa(o.RiskScore),
			o.Status,
			paidAt,
		})
	}
	w.Flush()
}

// OrdersExport GET /api/admin/orders/export?from=&to=&status=&format=csv
// (别名，与 reports/export 逻辑相同，保持 URL 语义一致)
func (h *Handler) OrdersExport(c *gin.Context) {
	h.ReportsExport(c)
}

// OrdersStats GET /api/admin/orders/stats
func (h *Handler) OrdersStats(c *gin.Context) {
	type row struct {
		Status string  `json:"status"`
		Count  int64   `json:"count"`
		Amount float64 `json:"amount"`
	}
	var rows []row
	h.DB.Raw(`
		SELECT status, COUNT(*) AS count, COALESCE(SUM(amount),0) AS amount
		FROM orders
		GROUP BY status
	`).Scan(&rows)

	// 今日统计
	today := time.Now().UTC().Truncate(24 * time.Hour)
	var todayOrders int64
	var todayRevenue float64
	h.DB.Model(&model.Order{}).Where("created_at >= ?", today).Count(&todayOrders)
	h.DB.Model(&model.Order{}).
		Where("status = ? AND created_at >= ?", "completed", today).
		Select("COALESCE(SUM(amount),0)").Scan(&todayRevenue)

	response.OK(c, gin.H{
		"by_status":     rows,
		"today_orders":  todayOrders,
		"today_revenue": todayRevenue,
	})
}
