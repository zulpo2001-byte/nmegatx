package model

import (
	"time"
)

// PaypalAccount 对应 paypal_accounts 表
// mode=email：直接构造 PayPal.me 链接，无需 API 凭据
// mode=rest：走 PayPal OAuth2 REST API
type PaypalAccount struct {
	ID                 uint      `gorm:"primaryKey"                                            json:"id"`
	UserID             uint      `gorm:"index:idx_paypal_user_state,priority:1"                json:"user_id"`
	Label              string    `gorm:"size:100"                                              json:"label"`
	Mode               string    `gorm:"size:10;default:email"                                 json:"mode"`           // email | rest
	Email              string    `gorm:"size:255"                                              json:"email"`
	PaypalMeUsername   string    `gorm:"size:128"                                              json:"paypalme_username"`
	ClientID           string    `gorm:"type:text"                                             json:"-"`
	ClientSecret       string    `gorm:"type:text"                                             json:"-"`

	// 沙盒
	Sandbox             bool   `gorm:"default:false"                                         json:"sandbox"`
	SandboxMode         bool   `gorm:"default:false"                                         json:"sandbox_mode"`
	SandboxEmail        string `gorm:"size:255"                                              json:"sandbox_email"`
	SandboxPaypalMeUsername string `gorm:"size:128"                                          json:"sandbox_paypalme_username"`
	SandboxClientID     string `gorm:"type:text"                                             json:"-"`
	SandboxClientSecret string `gorm:"type:text"                                             json:"-"`

	// 状态机
	AccountState string `gorm:"size:20;default:active;index:idx_paypal_user_state,priority:2" json:"account_state"` // active|paused|abandoned
	Enabled      bool   `gorm:"default:true"                                                    json:"enabled"`

	// 轮询策略
	PollOrder int    `gorm:"default:0"      json:"poll_order"`
	PollMode  string `gorm:"size:20;default:random" json:"poll_mode"` // random | sequence

	// 权重
	Weight      int     `gorm:"default:10"  json:"weight"`
	SmartWeight float64 `gorm:"type:numeric(6,2);default:50" json:"smart_weight"`

	// 统计
	TotalOrders  int64 `gorm:"default:0" json:"total_orders"`
	TotalSuccess int64 `gorm:"default:0" json:"total_success"`
	FailCount    int   `gorm:"default:0" json:"fail_count"`

	// 金额门槛
	MinAmount      float64 `gorm:"type:numeric(10,2);default:0"     json:"min_amount"`
	MaxAmount      float64 `gorm:"type:numeric(10,2);default:99999" json:"max_amount"`
	MaxOrders      int     `gorm:"default:0"                        json:"max_orders"`
	MaxAmountTotal float64 `gorm:"type:numeric(12,2);default:0"     json:"max_amount_total"`

	// 日限额
	DailyOrders    int       `gorm:"default:0"    json:"daily_orders"`
	DailyAmount    float64   `gorm:"type:numeric(12,2);default:0" json:"daily_amount"`
	DailyResetHour int       `gorm:"default:0"    json:"daily_reset_hour"`
	LastResetDate  *time.Time `gorm:"type:date"   json:"last_reset_date"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CanAcceptAmount 检查金额是否在此账号的接受范围内
func (p *PaypalAccount) CanAcceptAmount(amount float64) bool {
	if p.MinAmount > 0 && amount < p.MinAmount {
		return false
	}
	if p.MaxAmount > 0 && amount > p.MaxAmount {
		return false
	}
	return true
}

// WouldExceedThreshold 检查接受此笔订单后是否超过日限额
func (p *PaypalAccount) WouldExceedThreshold(amount float64) bool {
	if p.MaxOrders > 0 && p.DailyOrders >= p.MaxOrders {
		return true
	}
	if p.MaxAmountTotal > 0 && p.DailyAmount+amount > p.MaxAmountTotal {
		return true
	}
	return false
}

// ShouldResetToday 判断今天是否应该执行日重置（统一 UTC 日期字符串比较，避免时区问题）
func (p *PaypalAccount) ShouldResetToday() bool {
	now := time.Now().UTC()
	if now.Hour() < p.DailyResetHour {
		return false
	}
	if p.LastResetDate == nil {
		return true
	}
	todayStr := now.Format("2006-01-02")
	lastStr := p.LastResetDate.UTC().Format("2006-01-02")
	return lastStr < todayStr
}

// IsActive 是否处于可用状态
func (p *PaypalAccount) IsActive() bool {
	return p.Enabled && p.AccountState == "active"
}

// ActiveEmail 返回当前模式下的邮箱（沙盒/生产）
func (p *PaypalAccount) ActiveEmail() string {
	if p.SandboxMode && p.SandboxEmail != "" {
		return p.SandboxEmail
	}
	return p.Email
}

func (p *PaypalAccount) ActivePaypalMeUsername() string {
	if p.SandboxMode && p.SandboxPaypalMeUsername != "" {
		return p.SandboxPaypalMeUsername
	}
	return p.PaypalMeUsername
}
