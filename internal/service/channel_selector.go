package service

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"nme-v9/internal/model"
)

const maxAccountHops = 10

// SelectPaypalAccount 为指定用户（只从该用户自己的账号池）选一个可用的 PayPal 账号
// 多层筛选：金额区间 → 日重置 → 日限额 → 轮询策略 → 分布式锁二次校验 → 原子递增
func SelectPaypalAccount(db *gorm.DB, rdb *redis.Client, logger *zap.Logger, smartSvc *SmartRoutingService, userID uint, amount float64) (*model.PaypalAccount, error) {
	ctx := context.Background()

	for hop := 0; hop < maxAccountHops; hop++ {
		// 每次循环重新查，确保拿到最新状态
		var candidates []model.PaypalAccount
		if err := db.Where("user_id = ? AND enabled = true AND account_state = 'active'", userID).
			Order("poll_order asc, id asc").
			Find(&candidates).Error; err != nil {
			return nil, err
		}
		if len(candidates) == 0 {
			return nil, errors.New("no paypal account configured for this user")
		}

		// 过滤 1：金额区间
		var filtered []model.PaypalAccount
		for _, a := range candidates {
			if a.CanAcceptAmount(amount) {
				filtered = append(filtered, a)
			}
		}
		if len(filtered) == 0 {
			return nil, errors.New("no paypal account accepts this amount range")
		}

		// 触发日重置（可配置重置时刻，非固定0点）
		for i := range filtered {
			acc := &filtered[i]
			if acc.ShouldResetToday() {
				today := time.Now().UTC().Format("2006-01-02")
				t, _ := time.Parse("2006-01-02", today)
				db.Model(acc).Updates(map[string]any{
					"daily_orders":     0,
					"daily_amount":     0,
					"call_count_daily": 0,
					"last_reset_date":  t,
				})
				acc.DailyOrders = 0
				acc.DailyAmount = 0
			}
		}

		// 过滤 2：日限额
		var available []model.PaypalAccount
		for _, a := range filtered {
			if !a.WouldExceedThreshold(amount) && !a.WouldExceedCallLimit() {
				available = append(available, a)
			}
		}
		if len(available) == 0 {
			// 全部超限 → 检查是否触发 sprint reset
			if maybeSprintResetPaypal(db, logger, userID) {
				continue // sprint reset 成功，重新循环
			}
			return nil, errors.New("all paypal accounts exceeded daily limits")
		}

		// 选账号：sequence 模式取 poll_order 最小，random 走加权（静态×0.4 + 智能×0.6）
		var chosen *model.PaypalAccount
		for i := range available {
			if available[i].PollMode == "sequence" {
				chosen = &available[i]
				break
			}
		}
		if chosen == nil {
			chosen = paypalWeightedRandom(available, smartSvc)
		}

		// 分布式锁（5秒TTL），防并发重复选同一账号
		lockKey := fmt.Sprintf("lock:paypal:%d", chosen.ID)
		if rdb != nil {
			if ok, _ := rdb.SetNX(ctx, lockKey, "1", 5*time.Second).Result(); !ok {
				continue // 被其他请求抢占，换一个
			}
		}

		// 锁内二次校验（防并发超限）
		var fresh model.PaypalAccount
		if err := db.First(&fresh, chosen.ID).Error; err != nil {
			if rdb != nil {
				rdb.Del(ctx, lockKey)
			}
			continue
		}
		if fresh.ShouldResetToday() {
			today := time.Now().UTC().Format("2006-01-02")
			t, _ := time.Parse("2006-01-02", today)
			db.Model(&fresh).Updates(map[string]any{
				"daily_orders":     0,
				"daily_amount":     0,
				"call_count_daily": 0,
				"last_reset_date":  t,
			})
			fresh.DailyOrders = 0
			fresh.DailyAmount = 0
		}
		if fresh.WouldExceedThreshold(amount) {
			if rdb != nil {
				rdb.Del(ctx, lockKey) // 立即释放，不等 TTL
			}
			continue
		}

		// 立即释放锁（计数已完成，不需要继续持有）
		if rdb != nil {
			rdb.Del(ctx, lockKey)
		}

		// 异步记录 SmartRouting 指标
		if smartSvc != nil {
			go smartSvc.RecordRequest("paypal", int64(fresh.ID), fresh.Label, true, 0, true)
		}

		if logger != nil {
			logger.Info("PayPal account selected",
				zap.Uint("user_id", userID),
				zap.Uint("account_id", fresh.ID),
				zap.String("label", fresh.Label),
				zap.Float64("amount", amount),
			)
		}
		return &fresh, nil
	}

	return nil, errors.New("no available paypal account after max retries")
}

// SelectStripeConfig 为指定用户（只从该用户自己的配置池）选一个可用的 Stripe 配置
func SelectStripeConfig(db *gorm.DB, rdb *redis.Client, logger *zap.Logger, smartSvc *SmartRoutingService, userID uint, amount float64) (*model.StripeConfig, error) {
	ctx := context.Background()

	for hop := 0; hop < maxAccountHops; hop++ {
		var candidates []model.StripeConfig
		if err := db.Where("user_id = ? AND enabled = true AND account_state = 'active'", userID).
			Order("poll_order asc, id asc").
			Find(&candidates).Error; err != nil {
			return nil, err
		}
		if len(candidates) == 0 {
			return nil, errors.New("no stripe config configured for this user")
		}

		var filtered []model.StripeConfig
		for _, s := range candidates {
			if s.CanAcceptAmount(amount) {
				filtered = append(filtered, s)
			}
		}
		if len(filtered) == 0 {
			return nil, errors.New("no stripe config accepts this amount range")
		}

		for i := range filtered {
			cfg := &filtered[i]
			if cfg.ShouldResetToday() {
				today := time.Now().UTC().Format("2006-01-02")
				t, _ := time.Parse("2006-01-02", today)
				db.Model(cfg).Updates(map[string]any{
					"daily_orders":     0,
					"daily_amount":     0,
					"call_count_daily": 0,
					"last_reset_date":  t,
				})
				cfg.DailyOrders = 0
				cfg.DailyAmount = 0
			}
		}

		var available []model.StripeConfig
		for _, s := range filtered {
			if !s.WouldExceedThreshold(amount) && !s.WouldExceedCallLimit() {
				available = append(available, s)
			}
		}
		if len(available) == 0 {
			if maybeSprintResetStripe(db, logger, userID) {
				continue
			}
			return nil, errors.New("all stripe configs exceeded daily limits")
		}

		var chosen *model.StripeConfig
		for i := range available {
			if available[i].PollMode == "sequence" {
				chosen = &available[i]
				break
			}
		}
		if chosen == nil {
			chosen = stripeWeightedRandom(available, smartSvc)
		}

		lockKey := fmt.Sprintf("lock:stripe:%d", chosen.ID)
		if rdb != nil {
			if ok, _ := rdb.SetNX(ctx, lockKey, "1", 5*time.Second).Result(); !ok {
				continue
			}
		}

		var fresh model.StripeConfig
		if err := db.First(&fresh, chosen.ID).Error; err != nil {
			if rdb != nil {
				rdb.Del(ctx, lockKey)
			}
			continue
		}
		if fresh.ShouldResetToday() {
			today := time.Now().UTC().Format("2006-01-02")
			t, _ := time.Parse("2006-01-02", today)
			db.Model(&fresh).Updates(map[string]any{
				"daily_orders":     0,
				"daily_amount":     0,
				"call_count_daily": 0,
				"last_reset_date":  t,
			})
			fresh.DailyOrders = 0
			fresh.DailyAmount = 0
		}
		if fresh.WouldExceedThreshold(amount) {
			if rdb != nil {
				rdb.Del(ctx, lockKey)
			}
			continue
		}

		if rdb != nil {
			rdb.Del(ctx, lockKey)
		}

		if smartSvc != nil {
			go smartSvc.RecordRequest("stripe", int64(fresh.ID), fresh.Label, true, 0, true)
		}

		if logger != nil {
			logger.Info("Stripe config selected",
				zap.Uint("user_id", userID),
				zap.Uint("config_id", fresh.ID),
				zap.String("label", fresh.Label),
				zap.Float64("amount", amount),
			)
		}
		return &fresh, nil
	}

	return nil, errors.New("no available stripe config after max retries")
}

// maybeSprintResetPaypal 全部超限时，若 reset_mode=sprint 则归零重来，返回 true 表示已重置
func maybeSprintResetPaypal(db *gorm.DB, logger *zap.Logger, userID uint) bool {
	var setting model.GlobalSetting
	if err := db.Where("key = 'reset_mode'").First(&setting).Error; err != nil || setting.Value != "sprint" {
		return false
	}
	today := time.Now().UTC().Format("2006-01-02")
	t, _ := time.Parse("2006-01-02", today)
	result := db.Model(&model.PaypalAccount{}).
		Where("user_id = ? AND enabled = true AND account_state = 'active'", userID).
		Updates(map[string]any{
			"daily_orders":    0,
			"daily_amount":    0,
			"last_reset_date": t,
		})
	if result.RowsAffected > 0 && logger != nil {
		logger.Warn("Sprint reset: paypal accounts", zap.Uint("user_id", userID))
	}
	return result.RowsAffected > 0
}

func maybeSprintResetStripe(db *gorm.DB, logger *zap.Logger, userID uint) bool {
	var setting model.GlobalSetting
	if err := db.Where("key = 'reset_mode'").First(&setting).Error; err != nil || setting.Value != "sprint" {
		return false
	}
	today := time.Now().UTC().Format("2006-01-02")
	t, _ := time.Parse("2006-01-02", today)
	result := db.Model(&model.StripeConfig{}).
		Where("user_id = ? AND enabled = true AND account_state = 'active'", userID).
		Updates(map[string]any{
			"daily_orders":    0,
			"daily_amount":    0,
			"last_reset_date": t,
		})
	if result.RowsAffected > 0 && logger != nil {
		logger.Warn("Sprint reset: stripe configs", zap.Uint("user_id", userID))
	}
	return result.RowsAffected > 0
}

// paypalWeightedRandom 加权随机，融合静态权重和智能动态权重
func paypalWeightedRandom(list []model.PaypalAccount, smartSvc *SmartRoutingService) *model.PaypalAccount {
	total := 0.0
	weights := make([]float64, len(list))
	for i, a := range list {
		sw := a.SmartWeight
		if smartSvc != nil {
			sw = smartSvc.GetDynamicWeight("paypal", int64(a.ID))
		}
		weights[i] = float64(a.Weight)*0.4 + sw*0.6
		total += weights[i]
	}
	if total <= 0 {
		return &list[0]
	}
	r := rand.Float64() * total
	cum := 0.0
	for i := range list {
		cum += weights[i]
		if r <= cum {
			return &list[i]
		}
	}
	return &list[len(list)-1]
}

func stripeWeightedRandom(list []model.StripeConfig, smartSvc *SmartRoutingService) *model.StripeConfig {
	total := 0.0
	weights := make([]float64, len(list))
	for i, s := range list {
		sw := s.SmartWeight
		if smartSvc != nil {
			sw = smartSvc.GetDynamicWeight("stripe", int64(s.ID))
		}
		weights[i] = float64(s.Weight)*0.4 + sw*0.6
		total += weights[i]
	}
	if total <= 0 {
		return &list[0]
	}
	r := rand.Float64() * total
	cum := 0.0
	for i := range list {
		cum += weights[i]
		if r <= cum {
			return &list[i]
		}
	}
	return &list[len(list)-1]
}
