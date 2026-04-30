package model

import "time"

type APIKey struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"index" json:"user_id"`
	APIKey    string    `gorm:"size:128;uniqueIndex" json:"api_key"`
	Secret    string    `gorm:"size:255" json:"-"`
	Enabled   bool      `gorm:"index" json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
