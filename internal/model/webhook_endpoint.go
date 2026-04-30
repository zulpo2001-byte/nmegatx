package model

import "time"

// WebhookEndpoint A站/B站 Webhook 对接端点
// type=a：接收 A站下单后的回调通知
// type=b：B站主动调 NMEGateway 下单/查询
type WebhookEndpoint struct {
	ID            uint      `gorm:"primaryKey"                                            json:"id"`
	UserID        uint      `gorm:"index:idx_webhook_user_type_enabled,priority:1"        json:"user_id"`
	Type          string    `gorm:"size:1;index:idx_webhook_user_type_enabled,priority:2" json:"type"` // a|b
	Label         string    `gorm:"size:100"                                              json:"label"`
	URL           string    `gorm:"size:1024"                                             json:"url"`
	PaymentMethod string    `gorm:"size:20;default:all"                                   json:"payment_method"` // all|stripe|paypal
	Enabled       bool      `gorm:"index:idx_webhook_user_type_enabled,priority:3"        json:"enabled"`

	// A 站密钥
	SharedSecret string `gorm:"size:255" json:"-"`        // HMAC 签名验证密钥，不对外暴露
	AApiKey      string `gorm:"size:255;uniqueIndex"      json:"a_api_key,omitempty"`

	// B 站密钥
	BApiKey       string `gorm:"size:255;uniqueIndex"      json:"b_api_key,omitempty"`
	BSharedSecret string `gorm:"size:255"                  json:"-"` // B站 HMAC 签名密钥，不对外暴露

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
