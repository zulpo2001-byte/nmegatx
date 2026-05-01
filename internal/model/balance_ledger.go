package model

import "time"

type BalanceLedger struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	UserID     uint      `gorm:"index" json:"user_id"`
	OrderID    *uint     `gorm:"index" json:"order_id,omitempty"`
	Type       string    `gorm:"size:20;index" json:"type"` // recharge|channel_fee
	AmountUSD  float64   `gorm:"not null" json:"amount_usd"`
	BalanceUSD float64   `gorm:"not null" json:"balance_usd"`
	Note       string    `gorm:"size:255" json:"note"`
	CreatedAt  time.Time `json:"created_at"`
}
