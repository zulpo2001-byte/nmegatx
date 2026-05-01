package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"nmegateway/internal/model"
)

// AlertService 多渠道告警服务
// 支持 telegram | webhook | email(预留)
type AlertService struct {
	DB  *gorm.DB
	Log *zap.Logger
}

// Fire 触发告警：写 DB 记录 + 异步推送所有匹配渠道
func (a *AlertService) Fire(alertType, level, title, message string, ctx map[string]any) {
	if a.DB == nil {
		if a.Log != nil {
			a.Log.Warn("alert fired (no db)", zap.String("type", alertType), zap.String("msg", message))
		}
		return
	}

	ctxJSON := "{}"
	if ctx != nil {
		if b, err := json.Marshal(ctx); err == nil {
			ctxJSON = string(b)
		}
	}

	record := model.AlertRecord{
		Type:    alertType,
		Level:   level,
		Event:   alertType,
		Title:   title,
		Payload: fmt.Sprintf(`{"message":%q}`, message),
		Context: ctxJSON,
		Status:  "open",
	}
	if err := a.DB.Create(&record).Error; err != nil && a.Log != nil {
		a.Log.Error("failed to save alert record", zap.Error(err))
	}

	// 异步推送，不阻塞主流程
	go a.pushToChannels(level, title, message, ctx)
}

// ── 便捷触发方法 ──────────────────────────────────────────────────────────────

func (a *AlertService) NoChannel(userID uint, paymentMethod string) {
	a.Fire("no_channel", "critical",
		"无可用支付通道",
		fmt.Sprintf("用户 %d 的 %s 支付通道全部耗尽或不可用", userID, paymentMethod),
		map[string]any{"user_id": userID, "payment_method": paymentMethod},
	)
}

func (a *AlertService) ChannelIsolated(channelType string, channelID uint, label string, reason string) {
	a.Fire("channel_isolated", "warning",
		"支付通道自动熔断",
		fmt.Sprintf("%s 账号 [%s] 已自动熔断: %s", channelType, label, reason),
		map[string]any{"channel_type": channelType, "channel_id": channelID, "label": label, "reason": reason},
	)
}

func (a *AlertService) ChargebackThreshold(channelType string, channelID uint, label string, failCount int) {
	a.Fire("chargeback", "critical",
		"拒付阈值触发",
		fmt.Sprintf("%s 账号 [%s] 失败次数达 %d，已自动停用", channelType, label, failCount),
		map[string]any{"channel_type": channelType, "channel_id": channelID, "label": label, "fail_count": failCount},
	)
}

func (a *AlertService) HighRiskTransaction(orderID uint, aOrderID string, score int, reasons []string) {
	a.Fire("high_risk", "warning",
		"高风险交易拦截",
		fmt.Sprintf("订单 %s 风险评分 %d，已拦截", aOrderID, score),
		map[string]any{"order_id": orderID, "a_order_id": aOrderID, "score": score, "reasons": reasons},
	)
}

func (a *AlertService) Anomaly(desc string, ctx map[string]any) {
	a.Fire("anomaly", "warning", "异常行为检测", desc, ctx)
}

// ── 渠道推送 ─────────────────────────────────────────────────────────────────

func (a *AlertService) pushToChannels(level, title, message string, ctx map[string]any) {
	if a.DB == nil {
		return
	}
	var channels []model.AlertChannel
	a.DB.Where("enabled = true").Find(&channels)

	for _, ch := range channels {
		if !a.channelAcceptsLevel(ch, level) {
			continue
		}
		switch ch.Type {
		case "telegram":
			a.sendTelegram(ch, title, message, level)
		case "webhook":
			a.sendWebhook(ch, level, title, message, ctx)
		}
	}
}

// channelAcceptsLevel 检查渠道是否接收该级别的告警
func (a *AlertService) channelAcceptsLevel(ch model.AlertChannel, level string) bool {
	if ch.Levels == "" {
		return true // 无限制，接收所有
	}
	var levels []string
	if err := json.Unmarshal([]byte(ch.Levels), &levels); err != nil {
		return true
	}
	for _, l := range levels {
		if l == "all" || l == level {
			return true
		}
	}
	return false
}

func (a *AlertService) sendTelegram(ch model.AlertChannel, title, message, level string) {
	var cfg map[string]string
	if err := json.Unmarshal([]byte(ch.Config), &cfg); err != nil {
		return
	}
	botToken := cfg["bot_token"]
	chatID := cfg["chat_id"]
	if botToken == "" || chatID == "" {
		return
	}

	emoji := "⚠️"
	if level == "critical" {
		emoji = "🚨"
	} else if level == "info" {
		emoji = "ℹ️"
	}

	text := fmt.Sprintf("%s *%s*\n%s\n_%s_",
		emoji, title, message, time.Now().Format("2006-01-02 15:04:05"))

	payload := map[string]any{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "Markdown",
	}
	body, _ := json.Marshal(payload)
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil && a.Log != nil {
		a.Log.Error("telegram alert failed", zap.Error(err))
		return
	}
	if resp != nil {
		defer resp.Body.Close()
	}
}

func (a *AlertService) sendWebhook(ch model.AlertChannel, level, title, message string, ctx map[string]any) {
	var cfg map[string]any
	if err := json.Unmarshal([]byte(ch.Config), &cfg); err != nil {
		return
	}
	webhookURL, _ := cfg["url"].(string)
	if webhookURL == "" {
		// 兼容旧 target 字段
		webhookURL = ch.Target
	}
	if webhookURL == "" {
		return
	}

	payload := map[string]any{
		"level":   level,
		"title":   title,
		"message": message,
		"context": ctx,
		"time":    time.Now().Unix(),
	}
	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", webhookURL, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	// 可选自定义 headers
	if headers, ok := cfg["headers"].(map[string]any); ok {
		for k, v := range headers {
			if vs, ok := v.(string); ok {
				req.Header.Set(k, vs)
			}
		}
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil && a.Log != nil {
		a.Log.Error("webhook alert failed", zap.String("url", webhookURL), zap.Error(err))
		return
	}
	if resp != nil {
		defer resp.Body.Close()
	}
}

// Warn 兼容旧接口（用 zap.Logger 直接记日志）
func (a *AlertService) Warn(event string, fields ...zap.Field) {
	if a.Log == nil {
		return
	}
	a.Log.Warn(event, fields...)
}

// ── 状态流转辅助 ─────────────────────────────────────────────────────────────

// Acknowledge 确认告警
func (a *AlertService) Acknowledge(id uint, byWho string) error {
	now := time.Now()
	return a.DB.Model(&model.AlertRecord{}).Where("id = ?", id).Updates(map[string]any{
		"status":          "acknowledged",
		"acknowledged_by": byWho,
		"acknowledged_at": &now,
	}).Error
}

// Resolve 解决告警
func (a *AlertService) Resolve(id uint) error {
	now := time.Now()
	return a.DB.Model(&model.AlertRecord{}).Where("id = ?", id).Updates(map[string]any{
		"status":     "resolved",
		"resolved_at": &now,
	}).Error
}

// TestPush 向所有启用渠道发送测试告警
func (a *AlertService) TestPush() {
	a.pushToChannels("info", "🔔 测试告警", "这是一条测试消息，如果你看到它说明告警渠道配置正确。", nil)
}

// GetChannelList 获取所有告警渠道（隐藏敏感 config 字段）
func (a *AlertService) GetChannelList() []map[string]any {
	var channels []model.AlertChannel
	a.DB.Where("enabled = true").Find(&channels)
	var result []map[string]any
	for _, ch := range channels {
		// 隐藏 bot_token/secret 等敏感字段
		safeCfg := maskSensitiveConfig(ch.Config)
		result = append(result, map[string]any{
			"id":      ch.ID,
			"name":    ch.Name,
			"type":    ch.Type,
			"levels":  ch.Levels,
			"enabled": ch.Enabled,
			"config":  safeCfg,
		})
	}
	return result
}

func maskSensitiveConfig(cfg string) map[string]any {
	var m map[string]any
	if err := json.Unmarshal([]byte(cfg), &m); err != nil {
		return map[string]any{}
	}
	sensitiveKeys := []string{"bot_token", "secret", "password", "token"}
	for _, k := range sensitiveKeys {
		for mk := range m {
			if strings.Contains(strings.ToLower(mk), k) {
				m[mk] = "***"
			}
		}
	}
	return m
}
