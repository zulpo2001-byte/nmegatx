package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"nmegateway/internal/model"
)

// RiskResult 风控评估结果
type RiskResult struct {
	Score   int      `json:"score"`
	Level   string   `json:"level"`   // low | medium | high
	Blocked bool     `json:"blocked"`
	Reasons []string `json:"reasons"`
}

// AssessRisk 简化入口（兼容旧调用，无 Redis）
func AssessRisk(amount float64) (int, bool) {
	r := AssessRiskFull(nil, nil, amount, "", 0)
	return r.Score, r.Blocked
}

// AssessRiskWithDB 兼容旧调用
func AssessRiskWithDB(db *gorm.DB, amount float64, ip string) (int, bool) {
	r := AssessRiskFull(db, nil, amount, ip, 0)
	return r.Score, r.Blocked
}

// AssessRiskFull 完整三层风控评估
// 第一层：内置实时规则（Redis 计数）
// 第二层：DB 规则引擎（动态增删）
// 第三层：综合判定
func AssessRiskFull(db *gorm.DB, rdb *redis.Client, amount float64, ip string, userID uint) RiskResult {
	var reasons []string
	score := 0
	ctx := context.Background()

	// 先读取 DB 规则，避免和内置规则对同一维度双重叠加
	var rules []model.RiskRule
	hasIPFreqRule := false
	hasLargeAmountRule := false
	if db != nil {
		db.Where("enabled = true").Find(&rules)
		for _, rule := range rules {
			if rule.Type == "ip_frequency" {
				hasIPFreqRule = true
			}
			if rule.Type == "amount_range" {
				hasLargeAmountRule = true
			}
		}
	}

	// ── 第一层：内置实时规则 ────────────────────────────────────────

	// 1a. IP 频率：先检查当前计数，下单后由 IncrIPCount 递增
	// 这里只读不写，防止风控检查本身影响计数
	if !hasIPFreqRule && ip != "" && rdb != nil {
		ipKey := fmt.Sprintf("risk:ip_freq:%s", ip)
		cnt, _ := rdb.Get(ctx, ipKey).Int64()
		if cnt >= 5 {
			score += 30
			reasons = append(reasons, fmt.Sprintf("IP frequency too high (%d/min)", cnt+1))
		}
	}

	// 1b. 大额交易：> $1000 → +20
	if !hasLargeAmountRule && amount > 1000 {
		score += 20
		reasons = append(reasons, fmt.Sprintf("large amount: %.2f", amount))
	}

	// 1c. 用户日频率：当天 > 50 笔 → +25（只读，下单成功后由 IncrUserDailyCount 递增）
	if userID > 0 && rdb != nil {
		dayKey := fmt.Sprintf("risk:user_daily:%d:%s", userID, time.Now().UTC().Format("20060102"))
		cnt, _ := rdb.Get(ctx, dayKey).Int64()
		if cnt >= 50 {
			score += 25
			reasons = append(reasons, fmt.Sprintf("user daily frequency too high (%d today)", cnt+1))
		}
	}

	// ── 第二层：DB 规则引擎 ─────────────────────────────────────────
	if db != nil {
		dbScore := 0
		for _, rule := range rules {
			hit, reason := matchRule(rule, amount, ip, rdb)
			if !hit {
				continue
			}
			// 命中 → hit_count++（异步，不阻塞主流程）
			go db.Model(&model.RiskRule{}).Where("id = ?", rule.ID).
				UpdateColumn("hit_count", gorm.Expr("hit_count + 1"))

			if rule.Action == "block" {
				dbScore = 100
				reasons = append(reasons, fmt.Sprintf("rule[%s] block: %s", rule.Type, reason))
				break
			}
			// warn 类叠加风险分，上限 50
			add := rule.RiskScore
			if dbScore+add > 50 {
				add = 50 - dbScore
			}
			dbScore += add
			reasons = append(reasons, fmt.Sprintf("rule[%s]: %s (+%d)", rule.Type, reason, rule.RiskScore))
		}
		score += dbScore
	}

	// ── 第三层：综合判定 ────────────────────────────────────────────
	if score > 100 {
		score = 100
	}
	level := "low"
	if score >= 70 {
		level = "high"
	} else if score >= 40 {
		level = "medium"
	}

	return RiskResult{
		Score:   score,
		Level:   level,
		Blocked: score >= 70,
		Reasons: reasons,
	}
}

// matchRule 判断单条规则是否命中，返回 (hit, reason)
func matchRule(rule model.RiskRule, amount float64, ip string, rdb *redis.Client) (bool, string) {
	var cond map[string]any
	if rule.Conditions == "" {
		return false, ""
	}
	if err := json.Unmarshal([]byte(rule.Conditions), &cond); err != nil {
		return false, ""
	}

	switch rule.Type {
	case "amount_range":
		min, _ := cond["min"].(float64)
		max, _ := cond["max"].(float64)
		if max > 0 && amount >= min && amount <= max {
			return true, fmt.Sprintf("amount %.2f in [%.2f, %.2f]", amount, min, max)
		}

	case "ip_region":
		if ip == "" {
			return false, ""
		}
		prefixes, _ := cond["blocked_prefixes"].([]any)
		for _, p := range prefixes {
			prefix, _ := p.(string)
			if prefix != "" && strings.HasPrefix(ip, prefix) {
				return true, fmt.Sprintf("IP %s matches blocked prefix %s", ip, prefix)
			}
		}

	case "ip_frequency":
		if ip == "" || rdb == nil {
			return false, ""
		}
		maxPerMin := int64(5)
		if v, ok := cond["max_per_minute"].(float64); ok {
			maxPerMin = int64(v)
		}
		ipKey := fmt.Sprintf("risk:ip_freq:%s", ip)
		cnt, _ := rdb.Get(context.Background(), ipKey).Int64()
		if cnt > maxPerMin {
			return true, fmt.Sprintf("IP frequency %d > %d/min", cnt, maxPerMin)
		}
	}

	return false, ""
}

// IncrIPCount 下单成功后递增 IP 频率计数（与风控检查解耦）
func IncrIPCount(rdb *redis.Client, ip string) {
	if rdb == nil || ip == "" {
		return
	}
	ctx := context.Background()
	key := fmt.Sprintf("risk:ip_freq:%s", ip)
	rdb.Incr(ctx, key)
	rdb.Expire(ctx, key, 60*time.Second)
}

// IncrUserDailyCount 下单成功后递增用户日频率计数
func IncrUserDailyCount(rdb *redis.Client, userID uint) {
	if rdb == nil || userID == 0 {
		return
	}
	ctx := context.Background()
	key := fmt.Sprintf("risk:user_daily:%d:%s", userID, time.Now().UTC().Format("20060102"))
	rdb.Incr(ctx, key)
	rdb.Expire(ctx, key, 25*time.Hour)
}
