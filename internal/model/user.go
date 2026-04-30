package model

import "time"

type User struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	Email           string    `gorm:"uniqueIndex;size:191" json:"email"`
	Password        string    `json:"-"`
	Role            string    `gorm:"size:32;index" json:"role"`
	Status          string    `gorm:"size:32;index" json:"status"`
	Permissions     string    `gorm:"type:jsonb" json:"permissions"`
	BalanceUSD      float64   `gorm:"default:0" json:"balance_usd"`
	PaypalFeeRate   float64   `gorm:"default:0" json:"paypal_fee_rate"`
	StripeFeeRate   float64   `gorm:"default:0" json:"stripe_fee_rate"`
	ProductStrategy string    `gorm:"size:20;default:round_robin" json:"product_strategy"` // round_robin|random|fixed
	ExpiresAt       time.Time `json:"expires_at"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}
