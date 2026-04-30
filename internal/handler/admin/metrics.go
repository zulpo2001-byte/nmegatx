package admin

// metrics.go — 渠道监控指标（Redis 实时汇总）
//
// GET    /api/admin/metrics/summary               — 汇总所有端点的 B站请求 + Webhook 投递指标
// DELETE /api/admin/metrics/:endpoint_id/reset    — 清除某端点的 Redis 指标计数

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"nme-v9/internal/model"
	"nme-v9/internal/pkg/response"
)

// MetricsSummary GET /api/admin/metrics/summary
func (h *Handler) MetricsSummary(c *gin.Context) {
	if h.RDB == nil {
		response.Fail(c, http.StatusServiceUnavailable, "redis not available")
		return
	}
	ctx := context.Background()

	var endpoints []model.WebhookEndpoint
	h.DB.Where("enabled = true").Order("id asc").Find(&endpoints)

	type bstationItem struct {
		EndpointID    uint     `json:"endpoint_id"`
		URL           string   `json:"url"`
		PaymentMethod string   `json:"payment_method"`
		TotalRequests int64    `json:"total_requests"`
		SuccessCount  int64    `json:"success_count"`
		ErrorCount    int64    `json:"error_count"`
		SuccessRate   *float64 `json:"success_rate"`
		AvgLatencyMs  *int64   `json:"avg_latency_ms"`
		LastError     string   `json:"last_error,omitempty"`
		LastErrorAt   string   `json:"last_error_at,omitempty"`
	}

	type webhookItem struct {
		EndpointID      uint     `json:"endpoint_id"`
		URL             string   `json:"url"`
		PaymentMethod   string   `json:"payment_method"`
		TotalDispatched int64    `json:"total_dispatched"`
		TotalConfirmed  int64    `json:"total_confirmed"`
		TotalFailed     int64    `json:"total_failed"`
		Pending         int64    `json:"pending"`
		DeliveryRate    *float64 `json:"delivery_rate"`
		LastDispatchAt  string   `json:"last_dispatched_at,omitempty"`
	}

	var bItems []bstationItem
	var wItems []webhookItem
	var bTotalReq, bTotalSuccess, bTotalErr int64

	for _, ep := range endpoints {
		epIDStr := strconv.FormatUint(uint64(ep.ID), 10)

		// ── B站指标 ──────────────────────────────────────
		bKey := "nme:metrics:bstation:" + epIDStr
		bData, _ := h.RDB.HGetAll(ctx, bKey).Result()

		total := parseI64(bData["total_requests"])
		success := parseI64(bData["success_count"])
		errors := parseI64(bData["error_count"])
		latSum := parseI64(bData["total_latency_ms"])

		bi := bstationItem{
			EndpointID:    ep.ID,
			URL:           ep.URL,
			PaymentMethod: ep.PaymentMethod,
			TotalRequests: total,
			SuccessCount:  success,
			ErrorCount:    errors,
			LastError:     bData["last_error"],
			LastErrorAt:   bData["last_error_at"],
		}
		if total > 0 {
			rate := float64(success) / float64(total) * 100
			bi.SuccessRate = &rate
			lat := latSum / total
			bi.AvgLatencyMs = &lat
		}
		bItems = append(bItems, bi)
		bTotalReq += total
		bTotalSuccess += success
		bTotalErr += errors

		// ── Webhook 投递指标 ──────────────────────────────
		wKey := "nme:metrics:webhook:" + epIDStr
		wData, _ := h.RDB.HGetAll(ctx, wKey).Result()

		dispatched := parseI64(wData["total_dispatched"])
		confirmed := parseI64(wData["total_confirmed"])
		failed := parseI64(wData["total_failed"])
		pending := dispatched - confirmed - failed
		if pending < 0 {
			pending = 0
		}

		wi := webhookItem{
			EndpointID:      ep.ID,
			URL:             ep.URL,
			PaymentMethod:   ep.PaymentMethod,
			TotalDispatched: dispatched,
			TotalConfirmed:  confirmed,
			TotalFailed:     failed,
			Pending:         pending,
			LastDispatchAt:  wData["last_dispatched_at"],
		}
		if dispatched > 0 && (confirmed+failed) > 0 {
			rate := float64(confirmed) / float64(confirmed+failed) * 100
			wi.DeliveryRate = &rate
		}
		wItems = append(wItems, wi)
	}

	var bAvgSuccessPct float64
	if bTotalReq > 0 {
		bAvgSuccessPct = float64(bTotalSuccess) / float64(bTotalReq) * 100
	}

	response.OK(c, gin.H{
		"generated_at": time.Now().UTC().Format(time.RFC3339),
		"window":       "rolling",
		"bstation": gin.H{
			"endpoints":       bItems,
			"total_requests":  bTotalReq,
			"total_errors":    bTotalErr,
			"avg_success_pct": bAvgSuccessPct,
		},
		"webhook": gin.H{
			"endpoints": wItems,
		},
	})
}

// MetricsResetEndpoint DELETE /api/admin/metrics/:endpoint_id/reset
// 清除某端点在 Redis 中的所有指标计数
func (h *Handler) MetricsResetEndpoint(c *gin.Context) {
	if h.RDB == nil {
		response.Fail(c, http.StatusServiceUnavailable, "redis not available")
		return
	}
	epID := c.Param("endpoint_id")
	ctx := context.Background()
	bKey := "nme:metrics:bstation:" + epID
	wKey := "nme:metrics:webhook:" + epID
	h.RDB.Del(ctx, bKey, wKey)
	response.OK(c, gin.H{"endpoint_id": epID, "reset": true})
}

// parseI64 安全地将字符串解析为 int64，解析失败返回 0
func parseI64(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}
