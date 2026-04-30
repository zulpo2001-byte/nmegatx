package model

import "time"

type Order struct {
	ID              uint       `gorm:"primaryKey"                                              json:"id"`
	UserID          uint       `gorm:"index:idx_order_user_status,priority:1"                  json:"user_id"`
	AOrderID        string     `gorm:"size:128;index:idx_a_order_user,priority:1"              json:"a_order_id"`
	BOrderID        string     `gorm:"size:128;index"                                          json:"b_order_id"`
	BTransactionID  string     `gorm:"size:128"                                                json:"b_transaction_id"`
	PayToken        string     `gorm:"size:128;uniqueIndex"                                    json:"pay_token"`
	Amount          float64    `                                                               json:"amount"`
	PaymentURL      string     `gorm:"size:1024"                                               json:"payment_url"`
	ReturnURL       string     `gorm:"size:1024"                                               json:"return_url"`
	CheckoutURL     string     `gorm:"size:1024"                                               json:"checkout_url"`
	Status          string     `gorm:"size:32;index:idx_order_user_status,priority:2;index"    json:"status"`
	PaymentMethod   string     `gorm:"size:20;default:stripe"                                  json:"payment_method"` // stripe|paypal
	Currency        string     `gorm:"size:10;default:USD"                                     json:"currency"`
	Email           string     `gorm:"size:255"                                                json:"email"`
	IP              string     `gorm:"size:45"                                                 json:"ip"`
	RiskScore       int        `gorm:"default:0"                                               json:"risk_score"`
	PaypalAccountID *uint      `gorm:"index"                                                   json:"paypal_account_id"`
	StripeConfigID  *uint      `gorm:"index"                                                   json:"stripe_config_id"`
	GatewayLabel    string     `gorm:"size:100"                                                json:"gateway_label"`
	ValidityDays    *int       `gorm:"index"                                                   json:"validity_days"`
	ExpiresAt       *time.Time `gorm:"index"                                                   json:"expires_at"`
	PaidAt          *time.Time `                                                               json:"paid_at"`
	AbandonedAt     *time.Time `                                                               json:"abandoned_at"`
	CallbackState   string     `gorm:"size:32;index"                                           json:"callback_state"`
	CreatedAt       time.Time  `                                                               json:"created_at"`
	UpdatedAt       time.Time  `                                                               json:"updated_at"`
}
