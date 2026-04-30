package model

import "time"

// Role RBAC 角色
type Role struct {
	ID          uint      `gorm:"primaryKey"          json:"id"`
	Name        string    `gorm:"size:64;uniqueIndex" json:"name"`
	DisplayName string    `gorm:"size:100"            json:"display_name"`
	Description string    `gorm:"type:text"           json:"description"`
	Permissions string    `gorm:"type:jsonb"          json:"permissions"`
	CreatedAt   time.Time `                           json:"created_at"`
	UpdatedAt   time.Time `                           json:"updated_at"`
}

// RiskRule 风控规则：amount_range|ip_region|ip_frequency|user_frequency|device_fingerprint
type RiskRule struct {
	ID          uint      `gorm:"primaryKey"                      json:"id"`
	Name        string    `gorm:"size:128"                        json:"name"`
	Type        string    `gorm:"size:40;default:amount_range"    json:"type"`
	Enabled     bool      `                                       json:"enabled"`
	Action      string    `gorm:"size:20;default:block"           json:"action"`      // block|warn
	RiskScore   int       `gorm:"default:20"                      json:"risk_score"`
	Conditions  string    `gorm:"type:jsonb"                      json:"conditions"`
	Config      string    `gorm:"type:jsonb"                      json:"config"`
	HitCount    int64     `gorm:"default:0"                       json:"hit_count"`
	Description string    `gorm:"type:text"                       json:"description"`
	CreatedAt   time.Time `                                       json:"created_at"`
	UpdatedAt   time.Time `                                       json:"updated_at"`
}

// AlertRecord 告警记录：open → acknowledged → resolved
type AlertRecord struct {
	ID             uint       `gorm:"primaryKey"           json:"id"`
	Level          string     `gorm:"size:32"              json:"level"`   // info|warning|critical
	Type           string     `gorm:"size:40"              json:"type"`    // no_channel|channel_isolated|chargeback|high_risk|anomaly
	Event          string     `gorm:"size:128"             json:"event"`
	Title          string     `gorm:"size:255"             json:"title"`
	Payload        string     `gorm:"type:jsonb"           json:"payload"`
	Context        string     `gorm:"type:jsonb"           json:"context"`
	Status         string     `gorm:"size:32;default:open" json:"status"`           // open|acknowledged|resolved
	AcknowledgedBy string     `gorm:"size:100"             json:"acknowledged_by"`
	AcknowledgedAt *time.Time `                            json:"acknowledged_at"`
	ResolvedAt     *time.Time `                            json:"resolved_at"`
	CreatedAt      time.Time  `                            json:"created_at"`
}

// AlertChannel 告警推送渠道
type AlertChannel struct {
	ID        uint      `gorm:"primaryKey"            json:"id"`
	Name      string    `gorm:"size:100"              json:"name"`
	Type      string    `gorm:"size:32"               json:"type"`    // telegram|email|webhook
	Target    string    `gorm:"type:text"             json:"target"`  // 兼容旧字段
	Config    string    `gorm:"type:jsonb"            json:"config"`  // 渠道配置 JSON
	Levels    string    `gorm:"type:jsonb"            json:"levels"`  // ["warning","critical"] 或 ["all"]
	Enabled   bool      `gorm:"default:true"          json:"enabled"`
	CreatedAt time.Time `                             json:"created_at"`
}

// GlobalSetting 全局系统配置 key-value
type GlobalSetting struct {
	ID        uint      `gorm:"primaryKey"                  json:"id"`
	Key       string    `gorm:"size:128;uniqueIndex"        json:"key"`
	Value     string    `gorm:"type:text"                   json:"value"`
	CreatedAt time.Time `                                   json:"created_at"`
	UpdatedAt time.Time `                                   json:"updated_at"`
}

// AuditLog 操作审计日志
type AuditLog struct {
	ID        uint      `gorm:"primaryKey"    json:"id"`
	ActorID   *uint     `                     json:"actor_id"`
	Action    string    `gorm:"size:128"      json:"action"`
	Method    string    `gorm:"size:16"       json:"method"`
	Path      string    `                     json:"path"`
	IP        string    `gorm:"size:64"       json:"ip"`
	Payload   string    `gorm:"type:jsonb"    json:"payload"`
	CreatedAt time.Time `                     json:"created_at"`
}
