package model

import "time"

// ChannelMetric 对应 channel_metrics 表
// 每小时一条，记录 PayPal/Stripe 账号的性能数据，供 SmartRouting 计算动态权重
type ChannelMetric struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	ChannelType      string    `gorm:"size:20"    json:"channel_type"`     // paypal | stripe
	ChannelID        int64     `gorm:"index"      json:"channel_id"`
	ChannelLabel     string    `gorm:"size:100"   json:"channel_label"`
	HourSlot         time.Time `gorm:"index"      json:"hour_slot"`        // 精确到小时

	// 请求计数
	TotalRequests   int `gorm:"default:0" json:"total_requests"`
	SuccessCount    int `gorm:"default:0" json:"success_count"`
	FailCount       int `gorm:"default:0" json:"fail_count"`
	RiskRejectCount int `gorm:"default:0" json:"risk_reject_count"`

	// 性能
	AvgResponseMs int `gorm:"default:0" json:"avg_response_ms"`

	// 派生指标
	SuccessRate   float64 `gorm:"type:numeric(5,2);default:0"   json:"success_rate"`
	RiskPassRate  float64 `gorm:"type:numeric(5,2);default:100" json:"risk_pass_rate"`
	DynamicWeight float64 `gorm:"type:numeric(6,2);default:50"  json:"dynamic_weight"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
