package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"nmegateway/internal/model"
)

// SmartRoutingService 智能路由引擎
// 基于 channel_metrics 历史数据，每小时重算 PayPal/Stripe 账号的动态权重
// 公式：dynamic_weight = success_rate×0.50 + risk_pass_rate×0.30 + (1-norm_response)×0.20
type SmartRoutingService struct {
	DB     *gorm.DB
	RDB    *redis.Client
	Logger *zap.Logger
}

const (
	smartWeightCacheTTL = 60 * time.Minute
	metricBufFlushSize  = 100
)

// metricBuf Redis key 格式：metric_buf:{type}:{id}:{hour_slot}
type metricBufEntry struct {
	Total      int `json:"t"`
	Success    int `json:"s"`
	Fail       int `json:"f"`
	RiskReject int `json:"r"`
	ResponseMs int `json:"ms"`
}

// GetDynamicWeight 获取账号动态权重（优先从 Redis 缓存取，miss 则实时计算）
func (svc *SmartRoutingService) GetDynamicWeight(channelType string, channelID int64) float64 {
	if svc.RDB == nil {
		return 50.0
	}
	ctx := context.Background()
	cacheKey := fmt.Sprintf("smart_weight:%s:%d", channelType, channelID)
	val, err := svc.RDB.Get(ctx, cacheKey).Result()
	if err == nil {
		w, _ := strconv.ParseFloat(val, 64)
		if w > 0 {
			return w
		}
	}
	w := svc.computeWeight(channelType, channelID)
	svc.RDB.Set(ctx, cacheKey, fmt.Sprintf("%.2f", w), smartWeightCacheTTL)
	return w
}

// computeWeight 查过去 24h 的 channel_metrics，计算动态权重
func (svc *SmartRoutingService) computeWeight(channelType string, channelID int64) float64 {
	since := time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Hour)
	var metrics []model.ChannelMetric
	svc.DB.Where("channel_type = ? AND channel_id = ? AND hour_slot >= ?", channelType, channelID, since).
		Find(&metrics)

	if len(metrics) == 0 {
		return 50.0 // 无历史数据给默认中等权重
	}

	var totalReq, totalSuccess, totalFail, totalRiskReject, totalMs int
	for _, m := range metrics {
		totalReq += m.TotalRequests
		totalSuccess += m.SuccessCount
		totalFail += m.FailCount
		totalRiskReject += m.RiskRejectCount
		totalMs += m.AvgResponseMs * m.TotalRequests
	}

	if totalReq == 0 {
		return 50.0
	}

	successRate := float64(totalSuccess) / float64(totalReq) * 100
	riskPassRate := 100.0
	if totalReq-totalRiskReject >= 0 {
		riskPassRate = float64(totalReq-totalRiskReject) / float64(totalReq) * 100
	}
	avgMs := float64(totalMs) / float64(totalReq)
	// 归一化响应时间：以 3000ms 为上限
	normResponse := math.Min(avgMs/3000.0, 1.0)

	weight := successRate*0.50 + riskPassRate*0.30 + (1-normResponse)*20.0
	return math.Max(1.0, math.Min(100.0, weight))
}

// RecordRequest 记录一次请求结果到 Redis buffer
func (svc *SmartRoutingService) RecordRequest(channelType string, channelID int64, label string, success bool, responseMs int, riskPassed bool) {
	if svc.RDB == nil {
		return
	}
	ctx := context.Background()
	hourSlot := time.Now().UTC().Truncate(time.Hour).Format("2006010215")
	bufKey := fmt.Sprintf("metric_buf:%s:%d:%s", channelType, channelID, hourSlot)

	raw, err := svc.RDB.Get(ctx, bufKey).Bytes()
	var entry metricBufEntry
	if err == nil {
		_ = json.Unmarshal(raw, &entry)
	}

	entry.Total++
	entry.ResponseMs = (entry.ResponseMs*(entry.Total-1) + responseMs) / entry.Total
	if success {
		entry.Success++
	} else {
		entry.Fail++
	}
	if !riskPassed {
		entry.RiskReject++
	}

	data, _ := json.Marshal(entry)
	svc.RDB.Set(ctx, bufKey, data, 2*time.Hour)

	// 每积累 flushSize 次批量写 DB
	if entry.Total%metricBufFlushSize == 0 {
		go svc.flushMetricEntry(channelType, channelID, label, hourSlot, entry)
	}
}

// FlushAllMetrics 强制将所有 Redis buffer flush 到 DB（定时任务调用，防止低流量丢数据）
func (svc *SmartRoutingService) FlushAllMetrics() {
	if svc.RDB == nil {
		return
	}
	ctx := context.Background()
	keys, err := svc.RDB.Keys(ctx, "metric_buf:*").Result()
	if err != nil {
		return
	}
	for _, key := range keys {
		parts := strings.Split(key, ":")
		if len(parts) != 4 {
			continue
		}
		chType := parts[1]
		chID, _ := strconv.ParseInt(parts[2], 10, 64)
		hourSlot := parts[3]
		raw, err := svc.RDB.Get(ctx, key).Bytes()
		if err != nil {
			continue
		}
		var entry metricBufEntry
		if err := json.Unmarshal(raw, &entry); err != nil || entry.Total == 0 {
			continue
		}
		svc.flushMetricEntry(chType, chID, "", hourSlot, entry)
	}
}

func (svc *SmartRoutingService) flushMetricEntry(channelType string, channelID int64, label, hourSlot string, entry metricBufEntry) {
	t, err := time.ParseInLocation("2006010215", hourSlot, time.UTC)
	if err != nil {
		return
	}

	totalReq := entry.Total
	successRate := 0.0
	riskPassRate := 100.0
	if totalReq > 0 {
		successRate = float64(entry.Success) / float64(totalReq) * 100
		riskPassRate = float64(totalReq-entry.RiskReject) / float64(totalReq) * 100
	}

	svc.DB.Exec(`
		INSERT INTO channel_metrics
			(channel_type, channel_id, channel_label, hour_slot,
			 total_requests, success_count, fail_count, risk_reject_count,
			 avg_response_ms, success_rate, risk_pass_rate, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, now())
		ON CONFLICT (channel_type, channel_id, hour_slot)
		DO UPDATE SET
			total_requests    = channel_metrics.total_requests    + EXCLUDED.total_requests,
			success_count     = channel_metrics.success_count     + EXCLUDED.success_count,
			fail_count        = channel_metrics.fail_count        + EXCLUDED.fail_count,
			risk_reject_count = channel_metrics.risk_reject_count + EXCLUDED.risk_reject_count,
			avg_response_ms   = CASE
			    WHEN (channel_metrics.total_requests + EXCLUDED.total_requests) = 0 THEN 0
			    ELSE (channel_metrics.avg_response_ms * channel_metrics.total_requests
			          + EXCLUDED.avg_response_ms * EXCLUDED.total_requests)
			         / (channel_metrics.total_requests + EXCLUDED.total_requests)
			    END,
			success_rate      = EXCLUDED.success_rate,
			risk_pass_rate    = EXCLUDED.risk_pass_rate,
			updated_at        = now()
	`, channelType, channelID, label, t,
		totalReq, entry.Success, entry.Fail, entry.RiskReject,
		entry.ResponseMs, successRate, riskPassRate)
}

// RecalculateAll 批量重算所有活跃账号的动态权重，写回 smart_weight 字段
func (svc *SmartRoutingService) RecalculateAll() {
	svc.FlushAllMetrics()

	var paypalAccounts []model.PaypalAccount
	svc.DB.Where("enabled = true AND account_state = 'active'").Find(&paypalAccounts)
	for _, acc := range paypalAccounts {
		w := svc.computeWeight("paypal", int64(acc.ID))
		svc.DB.Model(&acc).Update("smart_weight", w)
		if svc.RDB != nil {
			ctx := context.Background()
			cacheKey := fmt.Sprintf("smart_weight:paypal:%d", acc.ID)
			svc.RDB.Set(ctx, cacheKey, fmt.Sprintf("%.2f", w), smartWeightCacheTTL)
		}
	}

	var stripeConfigs []model.StripeConfig
	svc.DB.Where("enabled = true AND account_state = 'active'").Find(&stripeConfigs)
	for _, cfg := range stripeConfigs {
		w := svc.computeWeight("stripe", int64(cfg.ID))
		svc.DB.Model(&cfg).Update("smart_weight", w)
		if svc.RDB != nil {
			ctx := context.Background()
			cacheKey := fmt.Sprintf("smart_weight:stripe:%d", cfg.ID)
			svc.RDB.Set(ctx, cacheKey, fmt.Sprintf("%.2f", w), smartWeightCacheTTL)
		}
	}

	if svc.Logger != nil {
		svc.Logger.Info("SmartRouting: recalculated all weights",
			zap.Int("paypal_count", len(paypalAccounts)),
			zap.Int("stripe_count", len(stripeConfigs)),
		)
	}
}

// WeightSnapshot 返回所有账号当前权重快照（供 Admin 查看）
type WeightSnapshot struct {
	ChannelType string  `json:"channel_type"`
	ChannelID   int64   `json:"channel_id"`
	Label       string  `json:"label"`
	StaticWeight int    `json:"static_weight"`
	SmartWeight  float64 `json:"smart_weight"`
	FinalWeight  float64 `json:"final_weight"` // static×0.4 + smart×0.6
}

func (svc *SmartRoutingService) GetWeightSnapshots(userID uint) []WeightSnapshot {
	var snapshots []WeightSnapshot

	var paypalAccounts []model.PaypalAccount
	svc.DB.Where("user_id = ? AND enabled = true AND account_state = 'active'", userID).Find(&paypalAccounts)
	for _, a := range paypalAccounts {
		snapshots = append(snapshots, WeightSnapshot{
			ChannelType:  "paypal",
			ChannelID:    int64(a.ID),
			Label:        a.Label,
			StaticWeight: a.Weight,
			SmartWeight:  a.SmartWeight,
			FinalWeight:  float64(a.Weight)*0.4 + a.SmartWeight*0.6,
		})
	}

	var stripeConfigs []model.StripeConfig
	svc.DB.Where("user_id = ? AND enabled = true AND account_state = 'active'", userID).Find(&stripeConfigs)
	for _, s := range stripeConfigs {
		snapshots = append(snapshots, WeightSnapshot{
			ChannelType:  "stripe",
			ChannelID:    int64(s.ID),
			Label:        s.Label,
			StaticWeight: s.Weight,
			SmartWeight:  s.SmartWeight,
			FinalWeight:  float64(s.Weight)*0.4 + s.SmartWeight*0.6,
		})
	}

	return snapshots
}
