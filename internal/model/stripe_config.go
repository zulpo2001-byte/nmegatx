package model

import "time"

// StripeConfig 对应 stripe_configs 表
type StripeConfig struct {
	ID             uint      `gorm:"primaryKey"                                             json:"id"`
	UserID         uint      `gorm:"index:idx_stripe_user_state,priority:1"                 json:"user_id"`
	Label          string    `gorm:"size:100"                                               json:"label"`
	SecretKey      string    `gorm:"type:text"                                              json:"-"`
	PublishableKey string    `gorm:"type:text"                                              json:"publishable_key"`
	WebhookSecret  string    `gorm:"type:text"                                              json:"-"`
	Sandbox        bool      `gorm:"default:false"                                          json:"sandbox"`

	// 状态机
	AccountState string `gorm:"size:20;default:active;index:idx_stripe_user_state,priority:2" json:"account_state"` // active|paused|abandoned
	Enabled      bool   `gorm:"default:true"                                                    json:"enabled"`

	// 轮询策略
	PollOrder int    `gorm:"default:0"              json:"poll_order"`
	PollMode  string `gorm:"size:20;default:random" json:"poll_mode"` // random | sequence

	// 权重
	Weight      int     `gorm:"default:10"                   json:"weight"`
	SmartWeight float64 `gorm:"type:numeric(6,2);default:50" json:"smart_weight"`

	// 统计
	TotalOrders int64 `gorm:"default:0" json:"total_orders"`
	FailCount   int   `gorm:"default:0" json:"fail_count"`

	// 金额门槛
	MinAmount      float64 `gorm:"type:numeric(10,2);default:0"     json:"min_amount"`
	MaxAmount      float64 `gorm:"type:numeric(10,2);default:99999" json:"max_amount"`
	MaxOrders      int     `gorm:"default:0"                        json:"max_orders"`
	MaxAmountTotal float64 `gorm:"type:numeric(12,2);default:0"     json:"max_amount_total"`

	// 日限额
	DailyOrders    int        `gorm:"default:0"                    json:"daily_orders"`
	DailyAmount    float64    `gorm:"type:numeric(12,2);default:0" json:"daily_amount"`
	DailyResetHour int        `gorm:"default:0"                    json:"daily_reset_hour"`
	LastResetDate  *time.Time `gorm:"type:date"                    json:"last_reset_date"`

	// 调用次数限制（0=不限）
	MaxCallsTotal  int64 `gorm:"default:0" json:"max_calls_total"`
	MaxCallsDaily  int64 `gorm:"default:0" json:"max_calls_daily"`
	CallCountTotal int64 `gorm:"default:0" json:"call_count_total"`
	CallCountDaily int64 `gorm:"default:0" json:"call_count_daily"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CanAcceptAmount 检查金额是否在接受范围内
func (s *StripeConfig) CanAcceptAmount(amount float64) bool {
	if s.MinAmount > 0 && amount < s.MinAmount {
		return false
	}
	if s.MaxAmount > 0 && amount > s.MaxAmount {
		return false
	}
	return true
}

// WouldExceedThreshold 接受此笔后是否超过日限额
func (s *StripeConfig) WouldExceedThreshold(amount float64) bool {
	if s.MaxOrders > 0 && s.DailyOrders >= s.MaxOrders {
		return true
	}
	if s.MaxAmountTotal > 0 && s.DailyAmount+amount > s.MaxAmountTotal {
		return true
	}
	return false
}

func (s *StripeConfig) WouldExceedCallLimit() bool {
	if s.MaxCallsTotal > 0 && s.CallCountTotal >= s.MaxCallsTotal {
		return true
	}
	if s.MaxCallsDaily > 0 && s.CallCountDaily >= s.MaxCallsDaily {
		return true
	}
	return false
}

// ShouldResetToday 判断是否应执行日重置（统一 UTC 日期字符串比较）
func (s *StripeConfig) ShouldResetToday() bool {
	now := time.Now().UTC()
	if now.Hour() < s.DailyResetHour {
		return false
	}
	if s.LastResetDate == nil {
		return true
	}
	todayStr := now.Format("2006-01-02")
	lastStr := s.LastResetDate.UTC().Format("2006-01-02")
	return lastStr < todayStr
}

// IsActive 是否处于可用状态
func (s *StripeConfig) IsActive() bool {
	return s.Enabled && s.AccountState == "active"
}
