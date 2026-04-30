package model

import "time"

// Product B站产品，只存 WooCommerce 产品 ID + 轮询策略参数
// 支付方式（Stripe/PayPal）由下单时 payment_method 参数决定，不在此处设置
type Product struct {
	ID         uint       `gorm:"primaryKey"                                              json:"id"`
	UserID     uint       `gorm:"index:idx_product_user_enabled,priority:1"               json:"user_id"`
	Label      string     `gorm:"size:100"                                                json:"label"`
	BProductID string     `gorm:"size:128"                                                json:"b_product_id"`
	Weight     int        `gorm:"default:1"                                               json:"weight"`
	PollOrder  int        `gorm:"index:idx_product_user_enabled,priority:3;default:0"     json:"poll_order"`
	Enabled    bool       `gorm:"index:idx_product_user_enabled,priority:2;default:true"  json:"enabled"`
	TotalUsed  int64      `gorm:"default:0"                                               json:"total_used"`
	LastUsedAt *time.Time `gorm:"index"                                                   json:"last_used_at"`
	CreatedAt  time.Time  `                                                               json:"created_at"`
	UpdatedAt  time.Time  `                                                               json:"updated_at"`
}
